package reqval

import (
	"github.com/gin-gonic/gin"
)

func BindGin(c *gin.Context, dst any) error {
	return BindCtx(c.Request.Context(), c.Request.Body, dst)
}

// AbortWithError writes the correct status/body for any of our error types:
//   - *AuthorizationError -> 403 {"error": msg}
//   - *ValidationError    -> 422 {"errors": {field: msg}}
//   - anything else       -> 400 {"error": msg}
func AbortWithError(c *gin.Context, err error) {
	if ae, ok := AsAuthorizationError(err); ok {
		c.AbortWithStatusJSON(ae.HTTPStatus(), gin.H{"error": ae.Message})
		return
	}
	if ve, ok := AsValidationError(err); ok {
		c.AbortWithStatusJSON(ve.HTTPStatus(), gin.H{"errors": ve.Fields})
		return
	}
	c.AbortWithStatusJSON(400, gin.H{"error": err.Error()})
}

// AbortWithValidationError is kept for backward compatibility.
func AbortWithValidationError(c *gin.Context, err error) {
	AbortWithError(c, err)
}
