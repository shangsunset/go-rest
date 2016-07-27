package rest

import (
  "io"
  "fmt"
  "time"
  "bytes"
  "strings"
  "io/ioutil"
  "net/http"
  "net/http/httptest"
  "encoding/json"
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
func (c *Context) HandleFunc(u string, f func(http.ResponseWriter, *Request, Pipeline)(interface{}, error)) *mux.Route {
  return c.Handle(u, c.pipeline.Add(HandlerFunc(f)))
}

/**
 * Create a route
 */
func (c *Context) Handle(u string, h Handler) *mux.Route {
  return c.router.HandleFunc(u, func(rsp http.ResponseWriter, req *http.Request){
    c.handle(rsp, newRequest(req), h)
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
  
  // handle the request itself and finalize if needed
  res, err := h.ServeRequest(rsp, req, nil)
  if (req.flags & reqFlagFinalized) != reqFlagFinalized {
    c.sendResponse(rsp, req, res, err)
    alt.Debugf("%s: [%v] (%v) %s %s", c.service.name, req.Id, time.Since(start), req.Method, where)
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
  rsp.Header().Set("X-Request-Id", req.Id)
  if err == nil {
    c.sendEntity(rsp, req, http.StatusOK, nil, res)
  }else{
    switch cerr := err.(type) {
      case *Error:
        alt.Errorf("%s: %v", c.service.name, cerr.Cause)
        c.sendEntity(rsp, req, cerr.Status, cerr.Headers, cerr.Cause)
      default:
        alt.Errorf("%s: %v", c.service.name, err)
        c.sendEntity(rsp, req, http.StatusInternalServerError, nil, basicError{http.StatusInternalServerError, err.Error()})
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
