package rest

import (
  "io"
  "fmt"
  "bytes"
  "strings"
  "io/ioutil"
  "encoding/json"
)

import (
  "net/http"
  "net/http/httptest"
  "github.com/gorilla/mux"
  "github.com/bww/go-alert"
)

/**
 * Internal request flags
 */
type requestFlags uint32
const (
  reqFlagNone         = 0
  reqFlagFinalized    = 1 << 0
)

/**
 * A service request
 */
type Request struct {
  *http.Request
  Attrs map[string]interface{}
  flags requestFlags
}

/**
 * Create a service request
 */
func newRequest(r *http.Request) *Request {
  return &Request{r, make(map[string]interface{}), 0}
}

/**
 * Finalize the request
 */
func (r *Request) Finalize() {
  r.flags |= reqFlagFinalized
}

/**
 * Request resource
 */
func (r *Request) Resource() string {
  if q := r.URL.Query(); q != nil && len(q) > 0 {
    return fmt.Sprintf("%s?%v", r.URL.Path, q.Encode())
  }else{
    return r.URL.Path
  }
}

/**
 * Requst handler
 */
type Handler func(http.ResponseWriter, *Request)(interface{}, error)

/**
 * Request interceptor
 */
type Interceptor interface {
  Intercept(http.ResponseWriter, *Request)(error)
}

/**
 * A service context
 */
type Context struct {
  service   *Service
  router    *mux.Router
  intercept []Interceptor
}

/**
 * Create a context
 */
func newContext(s *Service, r *mux.Router, i []Interceptor) *Context {
  return &Context{s, r, i}
}

/**
 * Create a route
 */
func (c *Context) HandleFunc(p string, f Handler) *mux.Route {
  return c.router.HandleFunc(p, func(rsp http.ResponseWriter, req *http.Request){
    c.handle(rsp, newRequest(req), f)
  })
}

/**
 * Handle a request
 */
func (c *Context) handle(rsp http.ResponseWriter, req *Request, f Handler) {
  
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
            c.sendResponse(rsp, req, nil, NewError(http.StatusInternalServerError, err))
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
  
  // execute interceptors, if we have any
  if c.intercept != nil {
    for _, e := range c.intercept {
      err := e.Intercept(rsp, req)
      if err != nil {
        if (req.flags & reqFlagFinalized) != reqFlagFinalized {
          c.sendResponse(rsp, req, nil, err)
          return
        }
      }
    }
  }
  
  // handle the request itself and finalize if needed
  res, err := f(rsp, req)
  if (req.flags & reqFlagFinalized) != reqFlagFinalized {
    c.sendResponse(rsp, req, res, err)
    alt.Debugf("%s: [%v] (%v) %s %s", c.service.name, req.Id, duration, req.Method, where)
    if trace { // check for a trace and output the response
      recorder := httptest.NewRecorder()
      c.sendResponse(recorder, req, res, err)
      
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

/**
 * Send a result
 */
func (c *Context) sendResponse(rsp http.ResponseWriter, req *Request, res interface{}, err error) {
  if err == nil {
    c.sendEntity(rsp, req, http.StatusOK, nil, res)
  }else{
    switch cerr := err.(type) {
      case *Error:
        alt.Errorf("%s: %v", c.service.name, cerr.Cause)
        c.sendEntity(rsp, req, cerr.Status, cerr.Headers, cerr.Cause)
      default:
        alt.Errorf("%s: %v", c.service.name, err)
        c.sendEntity(rsp, req, http.StatusInternalServerError, nil, err)
    }
  }
}

/**
 * Respond with an entity
 */
func (c *Context) sendEntity(rsp http.ResponseWriter, req *Request, status int, headers map[string]string, content interface{}) {
  
  if headers != nil {
    for k, v := range headers {
      rsp.Header().Add(k, v)
    }
  }
  
  if ua := c.service.userAgent; ua != "" {
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
        alt.Errorf("%s: Could not write entity: %v\nIn response to: %v %v\nEntity: %d bytes written", c.service.name, err, req.Method, req.URL, n)
        return
      }
      
    case json.RawMessage:
      rsp.Header().Add("Content-Type", "application/json")
      rsp.WriteHeader(status)
      
      _, err := rsp.Write([]byte(e))
      if err != nil {
        alt.Errorf("%s: Could not write entity: %v\nIn response to: %v %v\nEntity: %d bytes", c.service.name, err, req.Method, req.URL, len(e))
        return
      }
      
    default:
      rsp.Header().Add("Content-Type", "application/json")
      rsp.WriteHeader(status)
      
      data, err := json.Marshal(content)
      if err != nil {
        alt.Errorf("%s: Could not marshal entity: %v\nIn response to: %v %v", c.service.name, err, req.Method, req.URL)
        return
      }
      
      _, err = rsp.Write(data)
      if err != nil {
        alt.Errorf("%s: Could not write entity: %v\nIn response to: %v %v\nEntity: %d bytes", c.service.name, err, req.Method, req.URL, len(data))
        return
      }
      
  }
  
}
