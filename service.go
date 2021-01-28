package frame

import (
	"context"
	"errors"
	"fmt"
	"gocloud.dev/server"
	"gocloud.dev/server/health"
	"gocloud.dev/server/requestlog"
	"google.golang.org/grpc"
	"log"
	"net/http"
	"os"
)

const ctxKeyService = "serviceKey"

type Service struct {
	name           string
	server         *server.Server
	grpcServer     *grpc.Server
	queue          *Queue
	dataStore      *store
	healthCheckers []health.Checker
	cleanup        func()
}

type Option func(service *Service)

func NewService(name string, opts ...Option) *Service {

	defaultSrvOptions := &server.Options{
		RequestLogger: requestlog.NewNCSALogger(os.Stdout, func(e error) { fmt.Println(e) }),
		Driver:        &server.DefaultDriver{},
	}

	service := &Service{
		name:      name,
		server:    server.New(http.DefaultServeMux, defaultSrvOptions),
		dataStore: &store{},
		queue:     &Queue{},
	}

	for _, opt := range opts {
		opt(service)
	}

	return service
}

func ToContext(ctx context.Context, service *Service) context.Context {
	return context.WithValue(ctx, ctxKeyService, service)
}

func FromContext(ctx context.Context) *Service {
	service, ok := ctx.Value(ctxKeyService).(*Service)
	if !ok {
		return nil
	}

	return service
}

func (s *Service) AddCleanupMethod(f func()) {
	if s.cleanup == nil {
		s.cleanup = f
		return
	}

	old := s.cleanup
	s.cleanup = func() { old(); f() }
}

func (s *Service) AddHealthCheck(checker health.Checker) {
	if s.healthCheckers != nil {
		s.healthCheckers = []health.Checker{}
	}
	s.healthCheckers = append(s.healthCheckers, checker)
}

func (s *Service) Run(ctx context.Context, address string) error {

	if s.server == nil {
		return errors.New("attempting to run service without a server")
	}

	s.AddCleanupMethod(func() {
		err := s.server.Shutdown(ctx)
		if err != nil {
			log.Printf("Run -- Server could not shut down gracefully : %v", err)
		}
	})

	err := s.initPubsub(ctx)
	if err != nil {
		return err
	}

	err = s.server.ListenAndServe(address)
	return err

}

func (s *Service) Stop() {
	if s.cleanup != nil {
		s.cleanup()
	}
}
