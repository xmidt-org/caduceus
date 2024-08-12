package anclahelper

import (
	"context"
	"fmt"
	"os"

	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/caduceus/internal/sink"
	"github.com/xmidt-org/sallust"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type In struct {
	fx.In
	Svc      *ancla.ClientService
	Listener ancla.ListenerConfig
	Sink     sink.Wrapper
	LC       fx.Lifecycle
}

func InitializeAncla(in In) int {

	stopWatches, err := in.Svc.StartListener(in.Listener, setLoggerInContext(), in.Sink)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Webhook service start listener error: %v\n", err)
		return 1
	}
	in.LC.Append(fx.StopHook(stopWatches))
	return 0

}

func setLoggerInContext() func(context.Context, *zap.Logger) context.Context {
	return func(parent context.Context, logger *zap.Logger) context.Context {
		return sallust.With(parent, logger)
	}
}
