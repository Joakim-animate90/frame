package frame_test

import (
	"context"
	"errors"
	"github.com/pitabwire/frame"
	"testing"
)

func TestDefaultService(t *testing.T) {

	_, srv := frame.NewService("Test Srv")
	if srv == nil {
		t.Errorf("No default service could be instaniated")
		return
	}

	if srv.Name() != "Test Srv" {
		t.Errorf("s")
	}
}

func TestService(t *testing.T) {

	_, srv := frame.NewService("Test")
	if srv == nil {
		t.Errorf("No default service could be instaniated")
	}
}

func TestFromContext(t *testing.T) {

	ctx := context.Background()

	_, srv := frame.NewService("Test Srv")

	nullSrv := frame.FromContext(ctx)
	if nullSrv != nil {
		t.Errorf("Service was found in context yet it was not set")
	}

	ctx = frame.ToContext(ctx, srv)

	valueSrv := frame.FromContext(ctx)
	if valueSrv == nil {
		t.Errorf("No default service was found in context")
	}

}

func TestService_AddCleanupMethod(t *testing.T) {

	ctx, srv := frame.NewService("Test Srv")

	a := 30

	srv.AddCleanupMethod(func(ctx context.Context) {
		a++
	})

	srv.AddCleanupMethod(func(ctx context.Context) {
		a++
	})

	if a != 30 {
		t.Errorf("Clean up method is running prematurely")
	}

	srv.Stop(ctx)

	if a != 32 {
		t.Errorf("Clean up method is not running at shutdown")
	}
}

type testHC struct {
}

func (h *testHC) CheckHealth() error {
	return nil
}

func TestService_AddHealthCheck(t *testing.T) {

	_, srv := frame.NewService("Test Srv")

	healthChecker := new(testHC)

	if srv.HealthCheckers() != nil {
		t.Errorf("Health checkers are not supposed to be added by default")
	}

	srv.AddHealthCheck(healthChecker)

	if len(srv.HealthCheckers()) == 0 {
		t.Errorf("Health checkers are not being added to list")
	}
}

func TestBackGroundConsumer(t *testing.T) {

	ctx, srv := frame.NewService("Test Srv", frame.NoopDriver(), frame.BackGroundConsumer(func() error {
		return nil
	}))

	err := srv.Run(ctx, "")
	if err != nil {
		t.Errorf("could not start a background consumer peacefully : %v", err)
	}

	ctx, srv = frame.NewService("Test Srv", frame.BackGroundConsumer(func() error {
		return errors.New("background errors in the system")
	}))

	err = srv.Run(ctx, "9845")
	if err == nil {
		t.Errorf("could not propagate background consumer error correctly")
	}

}
