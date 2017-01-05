package rest

import (
  "fmt"
  "time"
  "strings"
  "net/http"
  "encoding/base64"
)

import (
  "bitbucket.org/madebymess/go-util"
)

/**
 * Attributes
 */
type Attrs map[string]interface{}

/**
 * Merge attributes
 */
func mergeAttrs(a ...Attrs) Attrs {
  var m Attrs
  if a != nil {
    m = make(Attrs)
    for _, e := range a {
      for k, v := range e {
        m[k] = v
      }
    }
  }
  return m
}

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
  Id    string
  Attrs Attrs
  flags requestFlags
  start time.Time
}

/**
 * Create a service request
 */
func newRequest(r *http.Request) *Request {
  return newRequestWithAttributes(r, nil)
}

/**
 * Create a service request
 */
func newRequestWithAttributes(r *http.Request, a Attrs) *Request {
  id := util.TimeUUID()
  return &Request{r, base64.RawURLEncoding.EncodeToString(id[:]), a, 0, time.Now()}
}

/**
 * Put attributes
 */
func (r *Request) putAttributes(a Attrs) {
  if r.Attrs == nil {
    r.Attrs = a
  }else{
    for k, v := range a {
      r.Attrs[k] = v
    }
  }
}

/**
 * Finalize the request
 */
func (r *Request) Finalize() {
  r.flags |= reqFlagFinalized
}

/**
 * Obtain the start / creation time of the request
 */
func (r *Request) Started() time.Time {
  return r.start
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
 * Determine if the specified content type is explicitly accepted
 */
func (r *Request) Accepts(ctype string) bool {
  h := r.Header.Get("Accept")
  if h != "" {
    parts := strings.Split(h, ",")
    for _, p := range parts {
      if strings.EqualFold(strings.TrimSpace(p), ctype) {
        return true
      }
    }
  }
  return false
}

/**
 * A handler pipeline
 */
type Pipeline []Handler

/**
 * Copy this pipeline, append a handler and return the copy
 */
func (p Pipeline) Add(h Handler) Pipeline {
  if p == nil {
    return Pipeline{h}
  }
  
  c := make(Pipeline, len(p))
  copy(c, p)
  
  switch v := h.(type) {
    case Pipeline:
      return append(c, v...) // flatten and append
    default:
      return append(c, v)
  }
}

/**
 * Continue processing the pipeline
 */
func (p Pipeline) Next(w http.ResponseWriter, r *Request) (interface{}, error) {
  if len(p) < 0 {
    return nil, nil // empty pipline
  }else{
    return p[0].ServeRequest(w, r, p[1:])
  }
}

/**
 * Serve a request
 */
func (p Pipeline) ServeRequest(w http.ResponseWriter, r *Request, x Pipeline) (interface{}, error) {
  return p.Next(w, r) // the parameter pipeline is ignored
}

/**
 * Requst handler
 */
type Handler interface {
  ServeRequest(http.ResponseWriter, *Request, Pipeline)(interface{}, error)
}

/**
 * Requst handler
 */
type HandlerFunc func(http.ResponseWriter, *Request, Pipeline)(interface{}, error)

/**
 * Serve a request
 */
func (h HandlerFunc) ServeRequest(w http.ResponseWriter, r *Request, p Pipeline) (interface{}, error) {
  return h(w, r, p)
}
