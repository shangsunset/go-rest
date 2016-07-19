package rest

import (
  "net/http"
)

/**
 * A request error
 */
type Error struct {
  Status    int
  Headers   map[string]string
  Cause     error
}

/**
 * Create a status error
 */
func NewError(s int, e error) *Error {
  return &Error{s, nil, e}
}

/**
 * Set headers
 */
func (e *Error) SetHeaders(h map[string]string) *Error {
  e.Headers = h
  return e
}

/**
 * Obtain the error message
 */
func (e Error) Error() string {
  if c := e.Cause; c != nil {
    return c.Error()
  }else{
    return http.StatusText(e.Status)
  }
}
