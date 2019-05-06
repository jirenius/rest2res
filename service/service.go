package service

import (
	"fmt"
	"strings"

	res "github.com/jirenius/go-res"
	"github.com/jirenius/resgate/logger"
	nats "github.com/nats-io/go-nats"
)

// A Service handles incoming requests from NATS Server and calls the
// appropriate callback on the resource handlers.
type Service struct {
	res    *res.Service
	nc     res.Conn      // NATS Server connection
	logger logger.Logger // Logger
	cfg    Config
}

// NewService creates a new rest2res service.
func NewService(cfg Config) (*Service, error) {
	s := &Service{
		res:    res.NewService(cfg.ServiceName),
		cfg:    cfg,
		logger: logger.NewStdLogger(false, false),
	}
	if err := s.addResources(); err != nil {
		return nil, err
	}

	return s, nil
}

// SetLogger sets the logger.
// Panics if service is already started.
func (s *Service) SetLogger(l logger.Logger) *Service {
	s.res.SetLogger(l)
	s.logger = l
	return s
}

// Logger returns the logger.
func (s *Service) Logger() logger.Logger {
	return s.logger
}

// Logf writes a formatted log message
func (s *Service) Logf(format string, v ...interface{}) {
	if s.logger == nil {
		return
	}
	s.logger.Logf("[Service] ", format, v...)
}

// Debugf writes a formatted debug message
func (s *Service) Debugf(format string, v ...interface{}) {
	if s.logger == nil {
		return
	}
	s.logger.Debugf("[Service] ", format, v...)
}

// Tracef writes a formatted trace message
func (s *Service) Tracef(format string, v ...interface{}) {
	if s.logger == nil {
		return
	}
	s.logger.Tracef("[Service] ", format, v...)
}

// ListenAndServe connects to the NATS server at the url. Once connected,
// it subscribes to incoming requests and serves them on a single goroutine
// in the order they are received. For each request, it calls the appropriate
// handler, or replies with the appropriate error if no handler is available.
//
// In case of disconnect, it will try to reconnect until Close is called,
// or until successfully reconnecting, upon which Reset will be called.
//
// ListenAndServe returns an error if failes to connect or subscribe.
// Otherwise, nil is returned once the connection is closed using Close.
func (s *Service) ListenAndServe(url string, options ...nats.Option) error {
	return s.res.ListenAndServe(url, options...)
}

// Serve subscribes to incoming requests on the *Conn nc, serving them on
// a single goroutine in the order they are received. For each request,
// it calls the appropriate handler, or replies with the appropriate
// error if no handler is available.
//
// Serve returns an error if failes to subscribe. Otherwise, nil is
// returned once the *Conn is closed.
func (s *Service) Serve(nc res.Conn) error {
	return s.res.Serve(nc)
}

// Shutdown closes any existing connection to NATS Server.
// Returns an error if service is not started.
func (s *Service) Shutdown() error {
	return s.res.Shutdown()
}

func (s *Service) addResources() error {
	for i := range s.cfg.Endpoints {
		cep := &s.cfg.Endpoints[i]

		ep, err := newEndpoint(s, cep)
		if err != nil {
			return fmt.Errorf("endpoint #%d is invalid: %s", i+1, err)
		}
		err = s.addResource(ep, cep.ResourceCfg, "", "")
		if err != nil {
			return fmt.Errorf("endpoint #%d has invalid config: %s", i+1, err)
		}
	}

	return nil
}

func (s *Service) addResource(ep *endpoint, r ResourceCfg, pattern, path string) error {
	if r.Pattern != "" {
		pattern = r.Pattern
	} else if r.Path != "" {
		pattern += "." + r.Path
	}

	if r.Path != "" {
		if path == "" {
			path = r.Path
		} else {
			path = "." + r.Path
		}
	}

	rid := s.cfg.ServiceName
	if pattern != "" {
		rid += "." + pattern
	}

	if err := ep.addPath(path, rid, ep.urlParams, r.Type, r.IDProp); err != nil {
		return err
	}

	ep.resetPatterns = append(ep.resetPatterns, resetPattern(rid, ep.urlParams))
	s.res.AddHandler(pattern, ep.handler())

	// Recursively add child resources
	for _, nr := range r.Resources {
		if err := s.addResource(ep, nr, pattern, path); err != nil {
			return err
		}
	}

	return nil
}

func urlParams(u string) ([]string, error) {
	var params []string
	var tagStart int
	var c byte
	l := len(u)
	i := 0

StateDefault:
	if i == l {
		return params, nil
	}
	if u[i] == '$' {
		i++
		if i == l {
			goto UnexpectedEnd
		}
		if u[i] != '{' {
			return nil, fmt.Errorf("expected character \"{\" at pos %d", i)
		}
		i++
		tagStart = i
		goto StateTag
	}
	i++
	goto StateDefault

StateTag:
	if i == l {
		goto UnexpectedEnd
	}
	c = u[i]
	if c == '}' {
		if i == tagStart {
			return nil, fmt.Errorf("empty tag at pos %d", i)
		}
		params = append(params, u[tagStart:i])
		i++
		goto StateDefault
	}
	if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
		return nil, fmt.Errorf("non alpha-numeric (a-z or 0-9) character in tag at pos %d", i)
	}
	i++
	goto StateTag

UnexpectedEnd:
	return nil, fmt.Errorf("unexpected end of tag")
}

func resetPattern(pattern string, urlParams []string) string {
	var tokens []string
	if pattern != "" {
		tokens = strings.Split(pattern, btsep)
	}
	for i, t := range tokens {
		if len(t) > 0 && t[0] == pmark {
			tt := t[1:]
			if containsString(urlParams, tt) {
				tokens[i] = "${" + tt + "}"
			} else {
				tokens[i] = "*"
			}
		}
	}
	return strings.Join(tokens, ".")
}
