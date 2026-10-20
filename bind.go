package reqval

import (
	"context"
	"encoding/json"
	"errors"
	"io"
)

var ErrEmptyBody = errors.New("reqval: empty request body")

func Bind(r io.Reader, dst any) error {
	return BindCtx(context.Background(), r, dst)
}

func BindCtx(ctx context.Context, r io.Reader, dst any) error {
	if err := runAuthorizer(ctx, dst); err != nil {
		return err
	}

	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return ErrEmptyBody
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return err
	}
	return ValidateCtx(ctx, dst)
}
