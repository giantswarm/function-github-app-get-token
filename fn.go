package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/crossplane/crossplane-runtime/pkg/errors"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/google/go-github/v66/github"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/giantswarm/function-github-app-get-token/input/v1beta1"
)

const (
	gitHost                      = "https://api.github.com"
	ourContextKey                = "apiextensions.crossplane.io/github-app-get-token"
	extraResourcesContextKey     = "apiextensions.crossplane.io/extra-resources"
	extraResourcesSecretValueKey = "credentials"
)

type AppAuth struct {
	Id             string `json:"id"`
	InstallationId string `json:"installation_id"`
	PemFile        string `json:"pem_file"`
}

type Credentials struct {
	AppAuths []AppAuth `json:"app_auth"`
	Owner    string    `json:"owner"`
}

type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer

	log logging.Logger
}

func (f *Function) RunFunction(_ context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	rsp := response.To(req, response.DefaultTTL)

	xr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, fmt.Sprintf("Failed to get observed XR for a function call: %v", err)))
		return rsp, err
	}

	log := f.log.WithValues(
		"xr-version", xr.Resource.GetAPIVersion(),
		"xr-kind", xr.Resource.GetKind(),
		"xr-name", xr.Resource.GetName(),
	)
	log.Debug("Starting new function invocation")

	in := &v1beta1.Input{}
	if err := request.GetInput(req, in); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get function's input"))
		return rsp, nil
	}
	if in.SecretKey == "" {
		response.Fatal(rsp, errors.New("context key needed to access the Secret with app's auth data can't be empty"))
		return rsp, nil
	}
	if in.ContextKey == "" {
		in.ContextKey = extraResourcesSecretValueKey
	}

	contextForSecret, ok := request.GetContextKey(req, extraResourcesContextKey)
	if !ok {
		err := fmt.Errorf("failed to get secret from context using key %s", extraResourcesContextKey)
		responseFatal(rsp, log, err)
		return rsp, err
	}
	base64EncSecret := contextForSecret.
		GetStructValue().Fields[in.SecretKey].
		GetListValue().Values[0].
		GetStructValue().Fields["data"].
		GetStructValue().Fields[in.ContextKey].
		GetStringValue()

	secretDataInt, err := base64.StdEncoding.DecodeString(base64EncSecret)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, fmt.Sprintf("failed to decode secret data from base64: %v", err)))
		return rsp, err
	}

	var creds Credentials
	if err := json.Unmarshal(secretDataInt, &creds); err != nil {
		responseFatal(rsp, log, errors.Wrap(err, fmt.Sprintf("failed to decode secret data from JSON: %v", err)))
		return rsp, err
	}
	appId, err := strconv.ParseInt(creds.AppAuths[0].Id, 10, 64)
	if err != nil {
		responseFatal(rsp, log, errors.Wrap(err, fmt.Sprintf("failed to parse App ID as int: %v", err)))
		return rsp, err
	}
	InstallationId, err := strconv.ParseInt(creds.AppAuths[0].InstallationId, 10, 64)
	if err != nil {
		responseFatal(rsp, log, errors.Wrap(err, fmt.Sprintf("failed to parse Installation ID as int: %v", err)))
		return rsp, err
	}
	privatePem := []byte(creds.AppAuths[0].PemFile)

	itr, err := ghinstallation.NewAppsTransport(http.DefaultTransport, appId, privatePem)
	if err != nil {
		responseFatal(rsp, log, errors.Wrap(err, fmt.Sprintf("Failed to create app transport: %v", err)))
		return rsp, err
	}

	// create git client with app transport
	itr.BaseURL = gitHost
	client, err := github.NewClient(
		&http.Client{
			Transport: itr,
			Timeout:   time.Second * 30,
		},
	).WithEnterpriseURLs(gitHost, gitHost)
	if err != nil {
		responseFatal(rsp, log, errors.Wrap(err, fmt.Sprintf("Failed to create git client for app: %v", err)))
		return rsp, err
	}

	token, _, err := client.Apps.CreateInstallationToken(
		context.Background(),
		InstallationId,
		&github.InstallationTokenOptions{})
	if err != nil {
		responseFatal(rsp, log, errors.Wrap(err, fmt.Sprintf("Failed to create installation token: %v", err)))
		return rsp, err
	}

	s := &structpb.Struct{}
	err = json.Unmarshal([]byte(fmt.Sprintf("{\"github-token\": \"%s\"}", token.GetToken())), s)
	if err != nil {
		responseFatal(rsp, log, errors.Wrap(err, fmt.Sprintf("Failed to unmarshal JSON response from GitHub: %v", err)))
		return rsp, err
	}

	response.SetContextKey(rsp, ourContextKey, structpb.NewStructValue(s))
	log.Debug("Token created")

	response.ConditionTrue(rsp, "FunctionSuccess", "Short-lived GitHub access token created and saved in context").
		TargetCompositeAndClaim()

	return rsp, nil
}

func responseFatal(rsp *fnv1.RunFunctionResponse, log logging.Logger, err error) {
	log.Info("Function failed", "error", err)
	response.Fatal(rsp, err)
}
