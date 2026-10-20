package reqval

import "github.com/gin-gonic/gin"

func BindGin(c *gin.Context, dst any) error {
	return BindCtx(c.Request.Context(), c.Request.Body, dst)
}

// AbortWithError writes the appropriate status and body.
// This is opt-in. The controller may instead inspect the error
// and choose its own response shape (JSON, flash, redirect).
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
