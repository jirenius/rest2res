package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	res "github.com/jirenius/go-res"
	"github.com/jirenius/timerqueue"
)

type endpoint struct {
	s             *Service
	url           string
	urlParams     []string
	refreshCount  int
	cachedURLs    map[string]*cachedResponse
	access        res.AccessHandler
	timeout       time.Duration
	group         string
	resetPatterns []string
	tq            *timerqueue.Queue
	mu            sync.RWMutex
	node
}

type cachedResponse struct {
	reloads   int
	reqParams map[string]string
	crs       map[string]cachedResource
	rerr      *res.Error
}

type cachedResource struct {
	typ        resourceType
	model      map[string]interface{}
	collection []interface{}
}

type resourceType byte

const defaultRefreshDuration = time.Second * 3

const (
	resourceTypeUnset resourceType = iota
	resourceTypeModel
	resourceTypeCollection
)

func newEndpoint(s *Service, cep *EndpointCfg) (*endpoint, error) {
	if cep.URL == "" {
		return nil, errors.New("missing url")
	}
	if cep.Pattern == "" {
		return nil, errors.New("missing pattern")
	}

	urlParams, err := urlParams(cep.URL)
	if err != nil {
		return nil, err
	}

	ep := &endpoint{
		s:            s,
		url:          cep.URL,
		urlParams:    urlParams,
		refreshCount: cep.RefreshCount,
		cachedURLs:   make(map[string]*cachedResponse),
		access:       cep.Access,
		timeout:      time.Millisecond * time.Duration(cep.Timeout),
	}
	ep.tq = timerqueue.New(ep.handleRefresh, time.Millisecond*time.Duration(cep.RefreshTime))

	return ep, nil
}

func (ep *endpoint) handler() res.Handler {
	return res.Handler{
		Access:      ep.access,
		GetResource: ep.getResource,
		Group:       ep.url,
	}
}

func (ep *endpoint) handleRefresh(i interface{}) {
	ep.s.Debugf("Refreshing %s", i)

	url := i.(string)

	// Check if url is cached
	ep.mu.RLock()
	cresp, ok := ep.cachedURLs[url]
	ep.mu.RUnlock()
	if !ok {
		ep.s.Logf("Url %s not found in cache on refresh", url)
		return
	}

	params := cresp.reqParams

	ep.s.res.WithGroup(url, func(s *res.Service) {
		cresp.reloads++
		if cresp.rerr != nil || cresp.reloads > ep.refreshCount {
			// Reset resources
			ep.mu.Lock()
			delete(ep.cachedURLs, url)
			ep.mu.Unlock()

			resetResources := make([]string, len(ep.resetPatterns))
			for i, rp := range ep.resetPatterns {
				for _, param := range ep.urlParams {
					rp = strings.Replace(rp, "${"+param+"}", params[param], 1)
				}
				resetResources[i] = rp
			}
			ep.s.res.Reset(resetResources, nil)
			return
		}

		defer ep.tq.Add(i)

		ncresp := ep.getURL(url, params)
		if ncresp.rerr != nil {
			ep.s.Logf("Error refreshing url %s:\n\t%s", url, ncresp.rerr.Message)
			return
		}

		for rid, nv := range ncresp.crs {
			v, ok := cresp.crs[rid]
			if ok {
				r, err := ep.s.res.Resource(rid)
				if err != nil {
					// This shouldn't be possible. Let's panic.
					panic(fmt.Sprintf("error getting res resource %s:\n\t%s", rid, err))
				}

				updateResource(v, nv, r)
				delete(cresp.crs, rid)
			}
		}

		// for rid := range cresp.crs {
		// 	r, err := ep.s.res.Resource(rid)
		// 	r.DeleteEvent()
		// }

		// Replacing the old cachedResources with the new ones
		cresp.crs = ncresp.crs
	})
}

func updateResource(v, nv cachedResource, r res.Resource) {
	switch v.typ {
	case resourceTypeModel:
		updateModel(v.model, nv.model, r)
	case resourceTypeCollection:
		updateCollection(v.collection, nv.collection, r)
	}
}

func updateModel(a, b map[string]interface{}, r res.Resource) {
	ch := make(map[string]interface{})
	for k := range a {
		if _, ok := b[k]; !ok {
			ch[k] = res.DeleteAction
		}
	}

	for k, v := range b {
		ov, ok := a[k]
		if !(ok && reflect.DeepEqual(v, ov)) {
			ch[k] = v
		}
	}

	r.ChangeEvent(ch)
}

func updateCollection(a, b []interface{}, r res.Resource) {
	var i, j int
	// Do a LCS matric calculation
	// https://en.wikipedia.org/wiki/Longest_common_subsequence_problem
	s := 0
	m := len(a)
	n := len(b)

	// Trim of matches at the start and end
	for s < m && s < n && reflect.DeepEqual(a[s], b[s]) {
		s++
	}

	if s == m && s == n {
		return
	}

	for s < m && s < n && reflect.DeepEqual(a[m-1], b[n-1]) {
		m--
		n--
	}

	var aa, bb []interface{}
	if s > 0 || m < len(a) {
		aa = a[s:m]
		m = m - s
	} else {
		aa = a
	}
	if s > 0 || n < len(b) {
		bb = b[s:n]
		n = n - s
	} else {
		bb = b
	}

	// Create matrix and initialize it
	w := m + 1
	c := make([]int, w*(n+1))

	for i = 0; i < m; i++ {
		for j = 0; j < n; j++ {
			if reflect.DeepEqual(aa[i], bb[j]) {
				c[(i+1)+w*(j+1)] = c[i+w*j] + 1
			} else {
				v1 := c[(i+1)+w*j]
				v2 := c[i+w*(j+1)]
				if v2 > v1 {
					c[(i+1)+w*(j+1)] = v2
				} else {
					c[(i+1)+w*(j+1)] = v1
				}
			}
		}
	}

	idx := m + s
	i = m
	j = n
	rm := 0

	var adds [][3]int
	addCount := n - c[w*(n+1)-1]
	if addCount > 0 {
		adds = make([][3]int, 0, addCount)
	}
Loop:
	for {
		m = i - 1
		n = j - 1
		switch {
		case i > 0 && j > 0 && reflect.DeepEqual(aa[m], bb[n]):
			idx--
			i--
			j--
		case j > 0 && (i == 0 || c[i+w*n] >= c[m+w*j]):
			adds = append(adds, [3]int{n, idx, rm})
			j--
		case i > 0 && (j == 0 || c[i+w*n] < c[m+w*j]):
			idx--
			r.RemoveEvent(idx)
			rm++
			i--
		default:
			break Loop
		}
	}

	// Do the adds
	l := len(adds) - 1
	for i := l; i >= 0; i-- {
		add := adds[i]
		r.AddEvent(bb[add[0]], add[1]-rm+add[2]+l-i)
	}
}

func (ep *endpoint) getResource(r res.GetRequest) {
	// Replace param placeholders
	url := ep.url
	for _, param := range ep.urlParams {
		url = strings.Replace(url, "${"+param+"}", r.PathParam(param), 1)
	}

	// Check if url is cached
	ep.mu.RLock()
	cresp, ok := ep.cachedURLs[url]
	ep.mu.RUnlock()
	if !ok {
		if ep.timeout > 0 {
			r.Timeout(ep.timeout)
		}
		cresp = ep.cacheURL(url, r.PathParams())
	}

	// Return any encountered error when getting the endpoint
	if cresp.rerr != nil {
		r.Error(cresp.rerr)
		return
	}

	// Check if resource exists
	cr, ok := cresp.crs[r.ResourceName()]
	if !ok {
		r.NotFound()
		return
	}

	switch cr.typ {
	case resourceTypeModel:
		r.Model(cr.model)
	case resourceTypeCollection:
		r.Collection(cr.collection)
	}
}

func (ep *endpoint) cacheURL(url string, reqParams map[string]string) *cachedResponse {
	cresp := ep.getURL(url, reqParams)
	ep.mu.Lock()
	ep.cachedURLs[url] = cresp
	ep.mu.Unlock()
	ep.tq.Add(url)

	return cresp
}

func (ep *endpoint) getURL(url string, reqParams map[string]string) *cachedResponse {
	cr := cachedResponse{reqParams: reqParams}
	// Make HTTP request
	resp, err := http.Get(url)
	if err != nil {
		ep.s.Debugf("Error fetching endpoint: %s\n\t%s", url, err)
		cr.rerr = res.InternalError(err)
		return &cr
	}
	defer resp.Body.Close()

	// Handle non-2XX status codes
	if resp.StatusCode == 404 {
		cr.rerr = res.ErrNotFound
		return &cr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		cr.rerr = res.InternalError(fmt.Errorf("unexpected response code: %d", resp.StatusCode))
		return &cr
	}

	// Read body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		cr.rerr = res.InternalError(err)
		return &cr
	}
	// Unmarshal body
	var v value
	if err = json.Unmarshal(body, &v); err != nil {
		cr.rerr = res.InternalError(err)
		return &cr
	}

	// Traverse the data
	crs := make(map[string]cachedResource)
	err = ep.traverse(crs, v, nil, reqParams)
	if err != nil {
		cr.rerr = res.InternalError(fmt.Errorf("invalid data structure for %s: %s", url, err))
		return &cr
	}

	cr.crs = crs
	return &cr
}

func (ep *endpoint) traverse(crs map[string]cachedResource, v value, path []string, reqParams map[string]string) error {
	var err error
	switch v.typ {
	case valueTypeObject:
		_, err = traverseModel(crs, v, path, &ep.node, reqParams, "")
	case valueTypeArray:
		_, err = traverseCollection(crs, v, path, &ep.node, reqParams, "")
	default:
		return errors.New("endpoint didn't respond with a json object or array")
	}
	if err != nil {
		return err
	}
	return nil
}

func traverseModel(crs map[string]cachedResource, v value, path []string, n *node, reqParams map[string]string, pathPart string) (res.Ref, error) {
	if n.typ != resourceTypeModel {
		return "", fmt.Errorf("expected a model at %s", pathStr(path))
	}

	// Append path part
	switch n.ptyp {
	case pathTypeDefault:
		path = append(path, pathPart)
	case pathTypeProperty:
		idv, ok := v.obj[n.idProp]
		if !ok {
			return "", fmt.Errorf("missing id property %s at:\n\t%s", n.idProp, pathStr(path))
		}
		switch idv.typ {
		case valueTypeString:
			var idstr string
			err := json.Unmarshal(idv.raw, &idstr)
			if err != nil {
				return "", err
			}
			path = append(path, idstr)
		case valueTypeNumber:
			path = append(path, string(idv.raw))
		default:
			return "", fmt.Errorf("invalid id value for property %s at:\n\t%s", n.idProp, pathStr(path))
		}
		path = append(path)
	}

	model := make(map[string]interface{})
	for k, kv := range v.obj {
		// Get next node
		next := n.nodes[k]
		if next == nil {
			next = n.param
		}

		switch kv.typ {
		case valueTypeObject:
			if next != nil {
				ref, err := traverseModel(crs, kv, path, next, reqParams, k)
				if err != nil {
					return "", err
				}
				model[k] = ref
			}
		case valueTypeArray:
			if next != nil {
				ref, err := traverseCollection(crs, kv, path, next, reqParams, k)
				if err != nil {
					return "", err
				}
				model[k] = ref
			}
		default:
			if next != nil {
				return "", fmt.Errorf("unexpected primitive value for property %s at %s", k, pathStr(path))
			}
			model[k] = kv
		}
	}

	// Create rid
	p := make([]interface{}, len(n.params))
	for j, pp := range n.params {
		switch pp.typ {
		case paramTypeURL:
			p[j] = reqParams[pp.name]
		case paramTypePath:
			p[j] = path[pp.idx]
		}
	}
	rid := fmt.Sprintf(n.pattern, p...)

	crs[rid] = cachedResource{
		typ:   resourceTypeModel,
		model: model,
	}
	return res.Ref(rid), nil
}

func traverseCollection(crs map[string]cachedResource, v value, path []string, n *node, reqParams map[string]string, pathPart string) (res.Ref, error) {
	if n.typ != resourceTypeCollection {
		return "", fmt.Errorf("expected a collection at %s", pathStr(path))
	}

	if n.ptyp != pathTypeRoot {
		// Append path part
		path = append(path, pathPart)
	}

	collection := make([]interface{}, len(v.arr))
	for j, kv := range v.arr {
		next := n.param

		switch kv.typ {
		case valueTypeObject:
			if next != nil {
				ref, err := traverseModel(crs, kv, path, next, reqParams, strconv.Itoa(j))
				if err != nil {
					return "", err
				}
				collection[j] = ref
			}
		case valueTypeArray:
			if next != nil {
				ref, err := traverseCollection(crs, kv, path, next, reqParams, strconv.Itoa(j))
				if err != nil {
					return "", err
				}
				collection[j] = ref
			}
		default:
			if next != nil {
				return "", fmt.Errorf("unexpected primitive value for element %d at %s", j, pathStr(path))
			}
			collection[j] = kv
		}
	}

	// Create rid
	p := make([]interface{}, len(n.params))
	for k, pp := range n.params {
		switch pp.typ {
		case paramTypeURL:
			p[k] = reqParams[pp.name]
		case paramTypePath:
			p[k] = path[pp.idx]
		}
	}
	rid := fmt.Sprintf(n.pattern, p...)

	crs[rid] = cachedResource{
		typ:        resourceTypeCollection,
		collection: collection,
	}
	return res.Ref(rid), nil
}

func pathStr(path []string) string {
	if len(path) == 0 {
		return "endpoint root"
	}
	return strings.Join(path, ".")
}
