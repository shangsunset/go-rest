package rest

import (
  "io"
  "fmt"
  "bytes"
  "net/http"
  "encoding/json"
)

/**
 * An entity
 */
type Entity interface {
  io.Reader
  ContentType()(string)
}

/**
 * A simple entity
 */
type BytesEntity struct {
  *bytes.Buffer
  contentType string
}

/**
 * Create a bytes entity
 */
func NewBytesEntity(t string, b []byte) *BytesEntity {
  return &BytesEntity{bytes.NewBuffer(b), t}
}

/**
 * Content type
 */
func (e BytesEntity) ContentType() string {
  return e.contentType
}

/**
 * An entity handler
 */
type EntityHandler func(http.ResponseWriter, *http.Request, int, interface{})(error)

/**
 * The default entity handler
 */
func DefaultEntityHandler(rsp http.ResponseWriter, req *http.Request, status int, content interface{}) error {
  switch e := content.(type) {
    
    case nil:
      rsp.WriteHeader(status)
    
    case Entity:
      rsp.Header().Add("Content-Type", e.ContentType())
      rsp.WriteHeader(status)
      
      n, err := io.Copy(rsp, e)
      if err != nil {
        return fmt.Errorf("Could not write entity: %v\nIn response to: %v %v\nEntity: %d bytes written", err, req.Method, req.URL, n)
      }
      
    case json.RawMessage:
      rsp.Header().Add("Content-Type", "application/json")
      rsp.WriteHeader(status)
      
      _, err := rsp.Write([]byte(e))
      if err != nil {
        return fmt.Errorf("Could not write entity: %v\nIn response to: %v %v\nEntity: %d bytes", err, req.Method, req.URL, len(e))
      }
      
    default:
      rsp.Header().Add("Content-Type", "application/json")
      rsp.WriteHeader(status)
      
      data, err := json.Marshal(content)
      if err != nil {
        return fmt.Errorf("Could not marshal entity: %v\nIn response to: %v %v", err, req.Method, req.URL)
      }
      
      _, err = rsp.Write(data)
      if err != nil {
        return fmt.Errorf("Could not write entity: %v\nIn response to: %v %v\nEntity: %d bytes", err, req.Method, req.URL, len(data))
      }
      
  }
  return nil
}
