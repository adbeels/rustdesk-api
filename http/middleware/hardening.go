package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// DefaultMaxBodySize se usa cuando gin.max-body-size no está configurado (4 MiB)
const DefaultMaxBodySize int64 = 4 << 20

// BodyLimit limita el tamaño del cuerpo de cualquier petición.
// Varios endpoints no piden sesión y leen el cuerpo completo a memoria (ShouldBindBodyWith).
func BodyLimit(max int64) gin.HandlerFunc {
	if max <= 0 {
		max = DefaultMaxBodySize
	}
	return func(c *gin.Context) {
		if c.Request.ContentLength > max {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Request body too large"})
			return
		}
		// Cubre los cuerpos sin Content-Length (chunked): la lectura falla al pasar el límite
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
		c.Next()
	}
}

// SecureHeaders agrega cabeceras básicas: la consola no se puede incrustar en otra página
// y el navegador no adivina tipos de contenido.
func SecureHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		h.Set("Referrer-Policy", "same-origin")
		c.Next()
	}
}
