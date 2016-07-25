package rest

import (
  "io"
  "bytes"
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
