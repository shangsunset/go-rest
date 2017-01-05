package rest

import (
  "io"
  "os"
  "fmt"
  "time"
  "regexp"
  "strings"
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
  EntityHandler EntityHandler
  Debug         bool
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
  pipeline      Pipeline
  traceRequests map[string]*regexp.Regexp
  entityHandler EntityHandler
  debug         bool
}

/**
 * Create a new service
 */
func NewService(c Config) *Service {
  
  s := &Service{}
  s.instance = c.Instance
  s.hostname = c.Hostname
  s.userAgent = c.UserAgent
  s.port = c.Endpoint
  s.router = mux.NewRouter()
  s.entityHandler = c.EntityHandler
  
  if c.Name == "" {
    s.name = "service"
  }else{
    s.name = c.Name
  }
  
  if c.Debug || os.Getenv("GOREST_DEBUG") == "true" {
    s.debug = true
  }
  
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
func (s *Service) Context() *Context {
  return newContext(s, s.router)
}

/**
 * Create a subrouter that can be configured for specialized use
 */
func (s *Service) Subrouter(p string) *mux.Router {
  return s.router.PathPrefix(p).Subrouter()
}

/**
 * Create a context scoped under a base path
 */
func (s *Service) ContextWithBasePath(p string) *Context {
  return newContext(s, s.router.PathPrefix(p).Subrouter())
}

/**
 * Attach a handler to the service pipeline
 */
func (s *Service) Use(h ...Handler) {
  if h != nil {
    for _, e := range h {
      s.pipeline = s.pipeline.Add(e)
    }
  }
}

/**
 * Run the service (this blocks forever)
 */
func (s *Service) Run() error {
  s.pipeline = s.pipeline.Add(HandlerFunc(s.routeRequest))
  
  server := &http.Server{
    Addr: s.port,
    Handler: s,
    ReadTimeout: 30 * time.Second,
    WriteTimeout: 30 * time.Second,
  }
  
  alt.Debugf("%s: Listening on %v", s.name, s.port)
  return server.ListenAndServe()
}

/**
 * Display all routes in the service
 */
func (s *Service) DumpRoutes(w io.Writer) error {
  return s.router.Walk(func(route *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
    p, err := route.GetPathTemplate()
    if err != nil {
      return err
    }
    fmt.Fprintf(w, "  %v", p)
    fmt.Fprintln(w)
    return nil
  })
  return nil
}

/**
 * Request handler
 */
func (s *Service) ServeHTTP(rsp http.ResponseWriter, req *http.Request) {
  wreq := newRequest(req)
  res, err := s.pipeline.Next(rsp, wreq)
  if res != nil || err != nil {
    s.sendResponse(rsp, wreq, res, err)
  }
}

/**
 * Default (routing) request handler; this is a bit weird, the context will
 * handle the result, so we return nothing from here
 */
func (s *Service) routeRequest(rsp http.ResponseWriter, req *Request, pln Pipeline) (interface{}, error) {
  s.router.ServeHTTP(rsp, req.Request)
  return nil, nil
}

/**
 * Send a result
 */
func (s *Service) sendResponse(rsp http.ResponseWriter, req *Request, res interface{}, err error) {
  rsp.Header().Set("X-Request-Id", req.Id)
  if err == nil {
    s.sendEntity(rsp, req, http.StatusOK, nil, res)
  }else{
    s.sendError(rsp, req, err)
  }
}

/**
 * Respond with an error
 */
func (s *Service) sendError(rsp http.ResponseWriter, req *Request, err error) {
  var r int
  var c error
  var h map[string]string
  
  switch cerr := err.(type) {
    case *Error:
      r = cerr.Status
      h = cerr.Headers
      c = cerr.Cause
      alt.Errorf("%s: [%v] %v", s.name, req.Id, cerr.Cause)
    default:
      r = http.StatusInternalServerError
      c = basicError{http.StatusInternalServerError, err.Error()}
      alt.Errorf("%s: [%v] %v", s.name, req.Id, err)
  }
  
  if req.Accepts("text/html") {
    s.sendEntity(rsp, req, r, h, htmlError(r, h, c))
  }else{
    s.sendEntity(rsp, req, r, h, c)
  }
}

/**
 * Respond with an entity
 */
func (s *Service) sendEntity(rsp http.ResponseWriter, req *Request, status int, headers map[string]string, content interface{}) {
  
  if headers != nil {
    for k, v := range headers {
      rsp.Header().Add(k, v)
    }
  }
  if ua := s.userAgent; ua != "" {
    rsp.Header().Add("User-Agent", ua)
  }
  
  var err error
  if s.entityHandler != nil {
    err = s.entityHandler(rsp, req, status, content)
  }else{
    err = DefaultEntityHandler(rsp, req, status, content)
  }
  if err != nil {
    alt.Errorf("%s: %v", s.name, err)
    return
  }
  
}

/**
 * Produce a HTML error entity
 */
func htmlError(status int, headers map[string]string, content error) Entity {
  
  e := content.Error()
  e  = strings.Replace(e, "&", "&amp;", -1)
  e  = strings.Replace(e, "<", "&lt;", -1)
  e  = strings.Replace(e, ">", "&gt;", -1)
  
  m := `<html><body>`
  m += `<h1>`+ fmt.Sprintf("%v %v", status, http.StatusText(status)) +`</h1>`
  m += `<p><pre>`+ e +`</pre></p>`
  m += `</body></html>`
  
  return NewBytesEntity("text/html", []byte(m))
}
