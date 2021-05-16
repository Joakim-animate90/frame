package frame

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"
)

func authorizationControlListWrite(ctx context.Context, action string, subject string) error {

	authorizationUrl := fmt.Sprintf("%s%s", GetEnv("KETO_AUTHORIZATION_WRITE_URL", ""), "/relation-tuples")
	authClaims := ClaimsFromContext(ctx)

	if authClaims == nil {
		return errors.New("only authenticated requsts should be used to check authorization")
	}

	payload := map[string]interface{}{
		"namespace": authClaims.TenantID,
		"object":    authClaims.PartitionID,
		"relation":  action,
		"subject":   subject,
	}

	err, _ := invokeACLService(ctx, http.MethodPut, authorizationUrl, payload)

	if err != nil {
		return err
	}

	return nil
}

func TestAuthorizationControlListWrite(t *testing.T) {

	err := os.Setenv("KETO_AUTHORIZATION_WRITE_URL", "http://localhost:4467")
	if err != nil {
		t.Errorf("Authorization write url was not setable %v", err)
		return
	}

	ctx := context.Background()
	srv := NewService("Test Srv")
	ctx = ToContext(ctx, srv)

	authClaim := AuthenticationClaims{
		TenantID: "tenant",
		PartitionID: "partition",
		ProfileID: "profile",
		AccessID: "access",
	}
	ctx = authClaim.ClaimsToContext(ctx)

	err = authorizationControlListWrite(ctx, "read", "tested")
	if err != nil {
		t.Errorf("Authorization write was not possible see %v", err)
		return
	}

}

func TestAuthorizationControlListHasAccess(t *testing.T) {

	err := os.Setenv("KETO_AUTHORIZATION_READ_URL", "http://localhost:4466")
	if err != nil {
		t.Errorf("Authorization read url was not setable %v", err)
		return
	}

	ctx := context.Background()
	srv := NewService("Test Srv")
	ctx = ToContext(ctx, srv)

	authClaim := AuthenticationClaims{
		TenantID: "tenant",
		PartitionID: "partition",
		ProfileID: "profile",
		AccessID: "access",
	}
	ctx = authClaim.ClaimsToContext(ctx)

	err = authorizationControlListWrite(ctx, "read", "reader")
	if err != nil {
		t.Errorf("Authorization write was not possible see %v", err)
		return
	}

	err, access := AuthorizationControlListHasAccess(ctx, "read", "reader")
	if err != nil {
		t.Errorf("Authorization check was not possible see %v", err)
	} else if !access {
		t.Errorf("Authorization check was forbidden")
		return
	}

	err, access = AuthorizationControlListHasAccess(ctx, "read", "read-master")
	if err != nil {
		t.Errorf("Authorization check was not possible see %v", err)
		return
	} else if access {
		t.Errorf("Authorization check was not forbidden yet shouldn't exist")
		return
	}

}
