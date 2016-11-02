package rest

import (
  "io"
  "os"
  "time"
  "regexp"
  "net/http"
  "encoding/json"
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
    switch cerr := err.(type) {
      case *Error:
        alt.Errorf("%s: [%v] %v", s.name, req.Id, cerr.Cause)
        s.sendEntity(rsp, req, cerr.Status, cerr.Headers, cerr.Cause)
      default:
        alt.Errorf("%s: [%v] %v", s.name, req.Id, err)
        s.sendEntity(rsp, req, http.StatusInternalServerError, nil, basicError{http.StatusInternalServerError, err.Error()})
    }
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
  
  switch e := content.(type) {
    
    case nil:
      rsp.WriteHeader(status)
    
    case Entity:
      rsp.Header().Add("Content-Type", e.ContentType())
      rsp.WriteHeader(status)
      
      n, err := io.Copy(rsp, e)
      if err != nil {
        alt.Errorf("%s: Could not write entity: %v\nIn response to: %v %v\nEntity: %d bytes written", s.name, err, req.Method, req.URL, n)
        return
      }
      
    case json.RawMessage:
      rsp.Header().Add("Content-Type", "application/json")
      rsp.WriteHeader(status)
      
      _, err := rsp.Write([]byte(e))
      if err != nil {
        alt.Errorf("%s: Could not write entity: %v\nIn response to: %v %v\nEntity: %d bytes", s.name, err, req.Method, req.URL, len(e))
        return
      }
      
    default:
      rsp.Header().Add("Content-Type", "application/json")
      rsp.WriteHeader(status)
      
      data, err := json.Marshal(content)
      if err != nil {
        alt.Errorf("%s: Could not marshal entity: %v\nIn response to: %v %v", s.name, err, req.Method, req.URL)
        return
      }
      
      _, err = rsp.Write(data)
      if err != nil {
        alt.Errorf("%s: Could not write entity: %v\nIn response to: %v %v\nEntity: %d bytes", s.name, err, req.Method, req.URL, len(data))
        return
      }
      
  }
  
}
