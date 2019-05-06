package service

import (
	"errors"
	"fmt"
	"strings"
)

const (
	pmark = '$'
	btsep = "."
)

var errInvalidPath = errors.New("invalid path")
var errInvalidPattern = errors.New("invalid pattern")

// A node represents one part of the path, and has pointers
// to the next nodes.
// Only one instance of handlers may exist per node.
type node struct {
	typ     resourceType
	nodes   map[string]*node
	param   *node
	pattern string
	params  []patternParam // pattern parameters
	ptyp    pathType
	idProp  string
}

// A pattern represent a parameter part of the resource name pattern.
type patternParam struct {
	typ  paramType
	name string // name of the parameter
	idx  int    // token index of the parameter for paramTypePath
}

type paramType byte

const (
	paramTypeUnset paramType = iota
	paramTypeURL
	paramTypePath
)

type pathType byte

const (
	pathTypeRoot pathType = iota
	pathTypeDefault
	pathTypeProperty
)

func (rn *node) addPath(path string, pattern string, urlParams []string, typStr string, idProp string) error {
	ptyp := pathTypeRoot
	var typ resourceType
	switch typStr {
	case "model":
		typ = resourceTypeModel
	case "collection":
		typ = resourceTypeCollection
	default:
		return fmt.Errorf("invalid resource type: %s", typStr)
	}

	// Parse the pattern to see what parameters we need to cover
	parsedPattern, params, err := parsePattern(pattern)
	if err != nil {
		return err
	}
	// Validate all URL parameters are covered, and set them
	for _, urlParam := range urlParams {
		j := patternParamsContain(params, urlParam)
		if j == -1 {
			return fmt.Errorf("param %s found in url but not in pattern:\n\t%s", urlParam, pattern)
		}
		params[j].typ = paramTypeURL
	}

	var tokens []string
	if path != "" {
		tokens = strings.Split(path, btsep)
	}

	l := rn

	var n *node

	for i, t := range tokens {
		ptyp = pathTypeDefault

		lt := len(t)
		if lt == 0 {
			return errInvalidPath
		}

		if t[0] == pmark {
			if lt == 1 {
				return errInvalidPath
			}
			name := t[1:]
			j := patternParamsContain(params, name)
			if j == -1 {
				return fmt.Errorf("param %s found in path:\n\t%s\nbut not in pattern:\n\t%s", name, path, pattern)
			}

			if params[j].typ != paramTypeUnset {
				return fmt.Errorf("param %s covered more than once in pattern:\n\t%s", name, pattern)
			}

			// Is it the last token?
			if i == len(tokens)-1 {
				switch l.typ {
				case resourceTypeModel:
				case resourceTypeCollection:
					// No ID property means we use index instead
					if idProp != "" {
						if typ != resourceTypeModel {
							return fmt.Errorf("idProp must only be used on model resources")
						}
						ptyp = pathTypeProperty
					}
				default:
					return fmt.Errorf("no parent resource set for path:\n\t%s", path)
				}
			}

			params[j].typ = paramTypePath
			params[j].idx = i

			if l.param == nil {
				l.param = &node{}
			}
			n = l.param
		} else {
			if l.nodes == nil {
				l.nodes = make(map[string]*node)
				n = &node{}
				l.nodes[t] = n
			} else {
				n = l.nodes[t]
				if n == nil {
					n = &node{}
					l.nodes[t] = n
				}
			}
		}

		l = n
	}

	if l.typ != resourceTypeUnset {
		return fmt.Errorf("registration already done for path:\n\t%s", path)
	}

	// Validate all pattern parameters are covered by path
	for _, p := range params {
		if p.typ == paramTypeUnset {
			return fmt.Errorf("missing pattern parameter %s in path:\n\t%s", p.name, path)
		}
	}

	l.typ = typ
	l.pattern = parsedPattern
	l.params = params
	l.ptyp = ptyp
	l.idProp = idProp

	return nil
}

func parsePattern(pattern string) (string, []patternParam, error) {
	var tokens []string
	if pattern != "" {
		tokens = strings.Split(pattern, btsep)
	}

	var params []patternParam
	for i, t := range tokens {
		lt := len(t)
		if lt == 0 {
			return "", nil, errInvalidPattern
		}

		if t[0] == pmark {
			if lt == 1 {
				return "", nil, errInvalidPattern
			}
			params = append(params, patternParam{
				typ:  paramTypeUnset,
				name: t[1:],
			})
			tokens[i] = "%s"
		}
	}

	return strings.Join(tokens, "."), params, nil
}

func containsString(a []string, s string) bool {
	for _, w := range a {
		if w == s {
			return true
		}
	}
	return false
}

// patternParamsContain searches for the first pattern param that contains
// the param name, and returns the index, or -1 if it was not found.
func patternParamsContain(pps []patternParam, name string) int {
	for i, pp := range pps {
		if pp.name == name {
			return i
		}
	}
	return -1
}
