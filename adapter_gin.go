package reqval

import "github.com/gin-gonic/gin"

func BindGin(c *gin.Context, dst any) error {
	return BindCtx(c.Request.Context(), c.Request.Body, dst)
}

func BindMultipartGin(c *gin.Context, dst any) error {
	return BindMultipartCtx(c.Request.Context(), c.Request, dst)
}

// BindFormGin parses application/x-www-form-urlencoded and validates.
func BindFormGin(c *gin.Context, dst any) error {
	return BindFormCtx(c.Request.Context(), c.Request, dst)
}

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
