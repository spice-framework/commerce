// Package intercept provides typed, reflection-free invocation decorators used
// by generated Spice boundaries.
package intercept

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// Invocation is one ordinary typed operation. Generated code supplies the
// direct method call as the terminal invocation.
type Invocation[Request, Response any] func(
	context.Context,
	Request,
) (Response, error)

// Interceptor wraps one typed invocation. It may inspect or replace the
// request/response, short-circuit, or call next. It owns no global registry and
// cannot select methods by string or reflection.
type Interceptor[Request, Response any] func(
	context.Context,
	Request,
	Invocation[Request, Response],
) (Response, error)

// Chain validates and freezes an interceptor chain. The first interceptor is
// outermost, matching ordinary nested Go function composition. The returned
// invocation rejects a nil context before user code executes.
func Chain[Request, Response any](
	terminal Invocation[Request, Response],
	interceptors ...Interceptor[Request, Response],
) (Invocation[Request, Response], error) {
	if terminal == nil {
		return nil, errors.New("construct interceptor chain: terminal invocation is nil")
	}
	owned := append([]Interceptor[Request, Response](nil), interceptors...)
	for index, interceptor := range owned {
		if interceptor == nil {
			return nil, fmt.Errorf(
				"construct interceptor chain: interceptor %d is nil",
				index,
			)
		}
	}
	invocation := terminal
	for _, interceptor := range slices.Backward(owned) {
		next := invocation
		invocation = func(
			ctx context.Context,
			request Request,
		) (Response, error) {
			return interceptor(ctx, request, next)
		}
	}
	return func(
		ctx context.Context,
		request Request,
	) (Response, error) {
		if ctx == nil {
			var zero Response
			return zero, errors.New("invoke interceptor chain: context is nil")
		}
		if err := ctx.Err(); err != nil {
			var zero Response
			return zero, fmt.Errorf("invoke interceptor chain: %w", err)
		}
		return invocation(ctx, request)
	}, nil
}
