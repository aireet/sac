package protobind

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"g.echo.tech/dev/sac/pkg/response"
)

var unmarshaler = protojson.UnmarshalOptions{DiscardUnknown: true}
var marshaler = protojson.MarshalOptions{EmitUnpopulated: false, UseProtoNames: true}

// Bind reads the request body and unmarshals it into a proto message using protojson.
// Returns true on success, false if an error response was already sent.
func Bind(c *gin.Context, msg proto.Message) bool {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.BadRequest(c, "Failed to read request body", err)
		return false
	}
	if err := unmarshaler.Unmarshal(body, msg); err != nil {
		response.BadRequest(c, "Invalid request body", err)
		return false
	}
	return true
}

// JSON marshals a proto message to JSON and writes it as the response.
func JSON(c *gin.Context, code int, msg proto.Message) {
	data, err := marshaler.Marshal(msg)
	if err != nil {
		response.InternalError(c, "Failed to marshal response", err)
		return
	}
	c.Data(code, "application/json", data)
}

// OK sends a 200 response with a proto message.
func OK(c *gin.Context, msg proto.Message) {
	JSON(c, http.StatusOK, msg)
}

// Created sends a 201 response with a proto message.
func Created(c *gin.Context, msg proto.Message) {
	JSON(c, http.StatusCreated, msg)
}
