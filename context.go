package rest

import (
  "fmt"
  "time"
  "bytes"
  "strings"
  "io/ioutil"
  "net/http"
  "net/http/httptest"
)

import (
  "github.com/gorilla/mux"
  "github.com/bww/go-alert"
)

/**
 * A service context
 */
type Context struct {
  service   *Service
  router    *mux.Router
  pipeline  Pipeline
}

/**
 * Create a context
 */
func newContext(s *Service, r *mux.Router) *Context {
  return &Context{s, r, nil}
}

/**
 * Attach a handler to the context pipeline
 */
func (c *Context) Use(h ...Handler) {
  if h != nil {
    for _, e := range h {
      c.pipeline = c.pipeline.Add(e)
    }
  }
}

/**
 * Create a route
 */
func (c *Context) HandleFunc(u string, f func(http.ResponseWriter, *Request, Pipeline)(interface{}, error), a ...Attrs) *mux.Route {
  return c.Handle(u, c.pipeline.Add(HandlerFunc(f)), a...)
}

/**
 * Create a route
 */
func (c *Context) Handle(u string, h Handler, a ...Attrs) *mux.Route {
  attr := mergeAttrs(a...)
  return c.router.HandleFunc(u, func(rsp http.ResponseWriter, req *http.Request){
    c.handle(rsp, newRequestWithAttributes(req, attr), h)
  })
}

/**
 * Handle a request
 */
func (c *Context) handle(rsp http.ResponseWriter, req *Request, h Handler) {
  start := time.Now()
  
  // deal with proxies
  if r := req.Header.Get("X-Forwarded-For"); r != "" {
    req.RemoteAddr = r
  }else if r = req.Header.Get("X-Origin-IP"); r != "" {
    req.RemoteAddr = r
  }
  
  // where is this request endpoint, including parameters
  var where string
  if q := req.URL.Query(); q != nil && len(q) > 0 {
    where = fmt.Sprintf("%s?%v", req.URL.Path, q.Encode())
  }else{
    where = req.URL.Path
  }
  
  // determine if we need to trace the request
  trace := false
  if c.service.traceRequests != nil && len(c.service.traceRequests) > 0 {
    for _, e := range c.service.traceRequests {
      if e.MatchString(req.URL.Path) {
        alt.Debugf("%s: [%s] (trace:%v) %s %s ", c.service.name, req.RemoteAddr, e, req.Method, where)
        
        if req.Header != nil {
          for k, v := range req.Header {
            if strings.EqualFold(k, "Authorization") {
              alt.Debugf("  < %v: <%v suppressed>", k, len(v))
            }else{
              alt.Debugf("  < %v: %v", k, v)
            }
          }
        }
        
        if req.Body != nil {
          data, err := ioutil.ReadAll(req.Body)
          if err != nil {
            c.service.sendResponse(rsp, req, nil, NewError(http.StatusInternalServerError, err))
            return 
          }
          alt.Debugf("  <")
          if data != nil && len(data) > 0 {
            alt.Debugf("  < %s", string(data))
          }
          req.Body = ioutil.NopCloser(bytes.NewBuffer(data))
        }
        
        alt.Debugf("  -")
        trace = true
        break
      }
    }
  }
  
  // handle the request itself and finalize if needed
  res, err := h.ServeRequest(rsp, req, nil)
  if (req.flags & reqFlagFinalized) != reqFlagFinalized {
    c.service.sendResponse(rsp, req, res, err)
    alt.Debugf("%s: [%v] (%v) %s %s", c.service.name, req.Id, time.Since(start), req.Method, where)
    if trace { // check for a trace and output the response
      recorder := httptest.NewRecorder()
      c.service.sendResponse(recorder, req, res, err)
      
      alt.Debugf("  > %v %s", recorder.Code, http.StatusText(recorder.Code))
      if recorder.HeaderMap != nil {
        for k, v := range recorder.HeaderMap {
          alt.Debugf("  > %v: %v", k, v)
        }
      }
      
      alt.Debugf("  >")
      if b := recorder.Body; b != nil {
        alt.Debugf("  > %v", string(b.Bytes()))
      }
      
      alt.Debugf("  #")
    }
  }
  
}
