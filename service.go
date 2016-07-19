package rest

import (
  "time"
  "regexp"
  "net/http"
)

import (
  "github.com/gorilla/mux"
  "github.com/bww/go-alert"
)

/**
 * Service config
 */
type Config struct {
  Name          string
  Instance      string
  Hostname      string
  UserAgent     string
  Endpoint      string
  TraceRegexps  []*regexp.Regexp
}

/**
 * A REST service
 */
type Service struct {
  name          string
  instance      string
  hostname      string
  userAgent     string
  port          string
  router        *mux.Router
  traceRequests map[string]*regexp.Regexp
  debug         bool
}

/**
 * Create a new service
 */
func NewService(c Config) *Service {
  
  s := &Service{}
  s.name = c.Name
  s.instance = c.Instance
  s.hostname = c.Hostname
  s.userAgent = c.UserAgent
  s.port = c.Endpoint
  s.router = mux.NewRouter()
  
  if c.TraceRegexps != nil {
    s.traceRequests = make(map[string]*regexp.Regexp)
    for _, e := range c.TraceRegexps {
      s.traceRequests[e.String()] = e
    }
  }
  
  return s
}

/**
 * Create a context
 */
func (s *Service) Context(i []Interceptor) *Context {
  return newContext(s, s.router.NewRoute().Subrouter(), i)
}

/**
 * Run the service (this blocks forever)
 */
func (s *Service) Run() error {
  
  server := &http.Server{
    Addr: s.port,
    Handler: s.router,
    ReadTimeout: 30 * time.Second,
    WriteTimeout: 30 * time.Second,
  }
  
  alt.Debugf("service: Listening for %v on %v", s.name, s.port)
  return server.ListenAndServe()
}
