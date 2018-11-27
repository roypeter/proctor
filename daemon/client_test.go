package daemon

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gojektech/proctor/cmd/version"

	"github.com/gojektech/proctor/config"
	"github.com/gojektech/proctor/io"
	"github.com/gorilla/websocket"
	"github.com/thingful/httpmock"

	"github.com/gojektech/proctor/proc/env"

	"github.com/gojektech/proctor/proc"
	"github.com/gojektech/proctor/proctord/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type TestConnectionError struct {
	message string
	timeout bool
}

func (e TestConnectionError) Error() string   { return e.message }
func (e TestConnectionError) Timeout() bool   { return e.timeout }
func (e TestConnectionError) Temporary() bool { return false }

func TestClientTestSuite(t *testing.T) {
	suite.Run(t, new(ClientTestSuite))
}

type ClientTestSuite struct {
	suite.Suite
	testClient       Client
	mockConfigLoader *config.MockLoader
	mockPrinter      *io.MockPrinter
}

func (s *ClientTestSuite) SetupTest() {
	s.mockConfigLoader = &config.MockLoader{}
	s.mockPrinter = &io.MockPrinter{}

	s.testClient = NewClient(s.mockPrinter, s.mockConfigLoader)
}

func (s *ClientTestSuite) TestListProcsReturnsListOfProcsWithDetails() {
	t := s.T()

	proctorConfig := config.ProctorConfig{Host: "proctor.example.com", Email: "proctor@example.com", AccessToken: "access-token"}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	body := `[ { "name": "job-1", "description": "job description", "image_name": "hub.docker.com/job-1:latest", "env_vars": { "secrets": [ { "name": "SECRET1", "description": "Base64 encoded secret for authentication." } ], "args": [ { "name": "ARG1", "description": "Argument name" } ] } } ]`
	var args = []env.VarMetadata{env.VarMetadata{Name: "ARG1", Description: "Argument name"}}
	var secrets = []env.VarMetadata{env.VarMetadata{Name: "SECRET1", Description: "Base64 encoded secret for authentication."}}
	envVars := env.Vars{Secrets: secrets, Args: args}
	var expectedProcList = []proc.Metadata{proc.Metadata{Name: "job-1", Description: "job description", EnvVars: envVars}}

	httpmock.RegisterStubRequest(
		httpmock.NewStubRequest(
			"GET",
			"http://"+proctorConfig.Host+"/jobs/metadata",
			func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(200, body), nil
			},
		).WithHeader(
			&http.Header{
				utility.UserEmailHeaderKey:     []string{"proctor@example.com"},
				utility.AccessTokenHeaderKey:   []string{"access-token"},
				utility.ClientVersionHeaderKey: []string{version.ClientVersion},
			},
		),
	)

	s.mockConfigLoader.On("Load").Return(proctorConfig, config.ConfigError{}).Once()

	procList, err := s.testClient.ListProcs()

	assert.NoError(t, err)
	s.mockConfigLoader.AssertExpectations(t)
	assert.Equal(t, expectedProcList, procList)
}

func (s *ClientTestSuite) TestListProcsReturnErrorFromResponseBody() {
	t := s.T()

	proctorConfig := config.ProctorConfig{Host: "proctor.example.com", Email: "proctor@example.com", AccessToken: "access-token"}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterStubRequest(
		httpmock.NewStubRequest(
			"GET",
			"http://"+proctorConfig.Host+"/jobs/metadata",
			func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(500, `{}`), nil
			},
		).WithHeader(
			&http.Header{
				utility.UserEmailHeaderKey:     []string{"proctor@example.com"},
				utility.AccessTokenHeaderKey:   []string{"access-token"},
				utility.ClientVersionHeaderKey: []string{version.ClientVersion},
			},
		),
	)

	s.mockConfigLoader.On("Load").Return(proctorConfig, config.ConfigError{}).Once()

	procList, err := s.testClient.ListProcs()

	assert.Equal(t, []proc.Metadata{}, procList)
	assert.Error(t, err)
	s.mockConfigLoader.AssertExpectations(t)
	assert.Equal(t, "Server Error!!!\nStatus Code: 500, Internal Server Error", err.Error())
}

func (s *ClientTestSuite) TestListProcsReturnClientSideTimeoutError() {
	t := s.T()

	proctorConfig := config.ProctorConfig{Host: "proctor.example.com", Email: "proctor@example.com", AccessToken: "access-token"}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterStubRequest(
		httpmock.NewStubRequest(
			"GET",
			"http://"+proctorConfig.Host+"/jobs/metadata",
			func(req *http.Request) (*http.Response, error) {
				return nil, TestConnectionError{message: "Unable to reach http://proctor.example.com/", timeout: true}
			},
		).WithHeader(
			&http.Header{
				utility.UserEmailHeaderKey:     []string{"proctor@example.com"},
				utility.AccessTokenHeaderKey:   []string{"access-token"},
				utility.ClientVersionHeaderKey: []string{version.ClientVersion},
			},
		),
	)

	s.mockConfigLoader.On("Load").Return(proctorConfig, config.ConfigError{}).Once()

	procList, err := s.testClient.ListProcs()

	assert.Equal(t, errors.New("Connection Timeout!!!\nGet http://proctor.example.com/jobs/metadata: Unable to reach http://proctor.example.com/\nPlease check your Internet/VPN connection for connectivity to ProctorD."), err)
	assert.Equal(t, []proc.Metadata{}, procList)
	s.mockConfigLoader.AssertExpectations(t)
}

func (s *ClientTestSuite) TestListProcsReturnClientSideConnectionError() {
	t := s.T()

	proctorConfig := config.ProctorConfig{Host: "proctor.example.com", Email: "proctor@example.com", AccessToken: "access-token"}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterStubRequest(
		httpmock.NewStubRequest(
			"GET",
			"http://"+proctorConfig.Host+"/jobs/metadata",
			func(req *http.Request) (*http.Response, error) {
				return nil, TestConnectionError{message: "Unknown Error", timeout: false}
			},
		).WithHeader(
			&http.Header{
				utility.UserEmailHeaderKey:     []string{"proctor@example.com"},
				utility.AccessTokenHeaderKey:   []string{"access-token"},
				utility.ClientVersionHeaderKey: []string{version.ClientVersion},
			},
		),
	)

	s.mockConfigLoader.On("Load").Return(proctorConfig, config.ConfigError{}).Once()

	procList, err := s.testClient.ListProcs()

	assert.Equal(t, errors.New("Network Error!!!\nGet http://proctor.example.com/jobs/metadata: Unknown Error"), err)
	assert.Equal(t, []proc.Metadata{}, procList)
	s.mockConfigLoader.AssertExpectations(t)
}

func (s *ClientTestSuite) TestListProcsForUnauthorizedUser() {
	t := s.T()

	proctorConfig := config.ProctorConfig{Host: "proctor.example.com", Email: "proctor@example.com", AccessToken: "access-token"}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterStubRequest(
		httpmock.NewStubRequest(
			"GET",
			"http://"+proctorConfig.Host+"/jobs/metadata",
			func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(401, `{}`), nil
			},
		).WithHeader(
			&http.Header{
				utility.UserEmailHeaderKey:     []string{"proctor@example.com"},
				utility.AccessTokenHeaderKey:   []string{"access-token"},
				utility.ClientVersionHeaderKey: []string{version.ClientVersion},
			},
		),
	)

	s.mockConfigLoader.On("Load").Return(proctorConfig, config.ConfigError{}).Once()

	procList, err := s.testClient.ListProcs()

	assert.Equal(t, []proc.Metadata{}, procList)
	assert.Equal(t, "Unauthorized Access!!!\nPlease check the EMAIL_ID and ACCESS_TOKEN validity in proctor config file.", err.Error())
	s.mockConfigLoader.AssertExpectations(t)
}

func (s *ClientTestSuite) TestListProcsForUnauthorizedErrorWithConfigMissing() {
	t := s.T()

	proctorConfig := config.ProctorConfig{Host: "proctor.example.com", Email: "proctor@example.com", AccessToken: ""}
	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterStubRequest(
		httpmock.NewStubRequest(
			"GET",
			"http://"+proctorConfig.Host+"/jobs/metadata",
			func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(401, `{}`), nil
			},
		).WithHeader(
			&http.Header{
				utility.UserEmailHeaderKey:     []string{"proctor@example.com"},
				utility.AccessTokenHeaderKey:   []string{""},
				utility.ClientVersionHeaderKey: []string{version.ClientVersion},
			},
		),
	)

	s.mockConfigLoader.On("Load").Return(proctorConfig, config.ConfigError{}).Once()
	procList, err := s.testClient.ListProcs()

	assert.Equal(t, []proc.Metadata{}, procList)
	assert.Equal(t, "Unauthorized Access!!!\nEMAIL_ID or ACCESS_TOKEN is not present in proctor config file.", err.Error())
	s.mockConfigLoader.AssertExpectations(t)
}

func (s *ClientTestSuite) TestExecuteProc() {
	t := s.T()

	proctorConfig := config.ProctorConfig{Host: "proctor.example.com", Email: "proctor@example.com", AccessToken: "access-token"}
	expectedProcResponse := "proctor-777b1dfb-ea27-46d9-b02c-839b75a542e2"
	body := `{ "name": "proctor-777b1dfb-ea27-46d9-b02c-839b75a542e2"}`
	procName := "run-sample"
	procArgs := map[string]string{"SAMPLE_ARG1": "sample-value"}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterStubRequest(
		httpmock.NewStubRequest(
			"POST",
			"http://"+proctorConfig.Host+"/jobs/execute",
			func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(201, body), nil
			},
		).WithHeader(
			&http.Header{
				utility.UserEmailHeaderKey:     []string{"proctor@example.com"},
				utility.AccessTokenHeaderKey:   []string{"access-token"},
				utility.ClientVersionHeaderKey: []string{version.ClientVersion},
				utility.ProcName:               []string{"run-sample"},
			},
		),
	)

	s.mockConfigLoader.On("Load").Return(proctorConfig, config.ConfigError{}).Once()

	executeProcResponse, err := s.testClient.ExecuteProc(procName, procArgs)

	assert.NoError(t, err)
	assert.Equal(t, expectedProcResponse, executeProcResponse)
	s.mockConfigLoader.AssertExpectations(t)
}

func (s *ClientTestSuite) TestExecuteProcInternalServerError() {
	t := s.T()
	proctorConfig := config.ProctorConfig{Host: "proctor.example.com", Email: "proctor@example.com", AccessToken: "access-token"}
	expectedProcResponse := ""
	procName := "run-sample"
	procArgs := map[string]string{"SAMPLE_ARG1": "sample-value"}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterStubRequest(
		httpmock.NewStubRequest(
			"POST",
			"http://"+proctorConfig.Host+"/jobs/execute",
			func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(500, ""), nil
			},
		).WithHeader(
			&http.Header{
				utility.UserEmailHeaderKey:     []string{"proctor@example.com"},
				utility.AccessTokenHeaderKey:   []string{"access-token"},
				utility.ClientVersionHeaderKey: []string{version.ClientVersion},
				utility.ProcName:               []string{"run-sample"},
			},
		),
	)

	s.mockConfigLoader.On("Load").Return(proctorConfig, config.ConfigError{}).Once()
	executeProcResponse, err := s.testClient.ExecuteProc(procName, procArgs)

	assert.Equal(t, "Server Error!!!\nStatus Code: 500, Internal Server Error", err.Error())
	assert.Equal(t, expectedProcResponse, executeProcResponse)
	s.mockConfigLoader.AssertExpectations(t)
}

func (s *ClientTestSuite) TestExecuteProcUnAuthorized() {
	t := s.T()
	proctorConfig := config.ProctorConfig{Host: "proctor.example.com", Email: "proctor@example.com", AccessToken: "access-token"}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterStubRequest(
		httpmock.NewStubRequest(
			"POST",
			"http://"+proctorConfig.Host+"/jobs/execute",
			func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(401, ""), nil
			},
		).WithHeader(
			&http.Header{
				utility.UserEmailHeaderKey:     []string{"proctor@example.com"},
				utility.AccessTokenHeaderKey:   []string{"access-token"},
				utility.ClientVersionHeaderKey: []string{version.ClientVersion},
				utility.ProcName:               []string{"run-sample"},
			},
		),
	)

	s.mockConfigLoader.On("Load").Return(proctorConfig, config.ConfigError{}).Once()

	executeProcResponse, err := s.testClient.ExecuteProc("run-sample", map[string]string{"SAMPLE_ARG1": "sample-value"})

	assert.Equal(t, "", executeProcResponse)
	assert.Equal(t, "Unauthorized Access!!!\nPlease check the EMAIL_ID and ACCESS_TOKEN validity in proctor config file.", err.Error())
	s.mockConfigLoader.AssertExpectations(t)
}

func (s *ClientTestSuite) TestExecuteProcUnAuthorizedWhenEmailAndAccessTokenNotSet() {
	t := s.T()
	proctorConfig := config.ProctorConfig{Host: "proctor.example.com"}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterStubRequest(
		httpmock.NewStubRequest(
			"POST",
			"http://"+proctorConfig.Host+"/jobs/execute",
			func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(401, ""), nil
			},
		).WithHeader(
			&http.Header{
				utility.UserEmailHeaderKey:     []string{""},
				utility.AccessTokenHeaderKey:   []string{""},
				utility.ClientVersionHeaderKey: []string{version.ClientVersion},
				utility.ProcName:               []string{"run-sample"},
			},
		),
	)

	s.mockConfigLoader.On("Load").Return(proctorConfig, config.ConfigError{}).Once()

	executeProcResponse, err := s.testClient.ExecuteProc("run-sample", map[string]string{"SAMPLE_ARG1": "sample-value"})

	assert.Equal(t, "", executeProcResponse)
	assert.Equal(t, "Unauthorized Access!!!\nEMAIL_ID or ACCESS_TOKEN is not present in proctor config file.", err.Error())
	s.mockConfigLoader.AssertExpectations(t)
}

func (s *ClientTestSuite) TestExecuteProcUnAuthorizedWhenUserIsNotAllowedToExecuteProc() {
	t := s.T()
	proctorConfig := config.ProctorConfig{Host: "proctor.example.com", Email: "proctor@example.com", AccessToken: "access-token"}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterStubRequest(
		httpmock.NewStubRequest(
			"POST",
			"http://"+proctorConfig.Host+"/jobs/execute",
			func(req *http.Request) (*http.Response, error) {
				return httpmock.NewStringResponse(403, ""), nil
			},
		).WithHeader(
			&http.Header{
				utility.UserEmailHeaderKey:     []string{"proctor@example.com"},
				utility.AccessTokenHeaderKey:   []string{"access-token"},
				utility.ClientVersionHeaderKey: []string{version.ClientVersion},
				utility.ProcName:               []string{"run-sample"},
			},
		),
	)

	s.mockConfigLoader.On("Load").Return(proctorConfig, config.ConfigError{}).Once()

	executeProcResponse, err := s.testClient.ExecuteProc("run-sample", map[string]string{"SAMPLE_ARG1": "sample-value"})

	assert.Equal(t, "", executeProcResponse)
	assert.Equal(t, "Access denied. You are not authorized to perform this action. Please contact proc admin.", err.Error())
	s.mockConfigLoader.AssertExpectations(t)
}

func (s *ClientTestSuite) TestExecuteProcsReturnClientSideConnectionError() {
	t := s.T()
	proctorConfig := config.ProctorConfig{Host: "proctor.example.com", Email: "proctor@example.com", AccessToken: "access-token"}

	httpmock.Activate()
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterStubRequest(
		httpmock.NewStubRequest(
			"POST",
			"http://"+proctorConfig.Host+"/jobs/execute",
			func(req *http.Request) (*http.Response, error) {
				return nil, TestConnectionError{message: "Unknown Error", timeout: false}
			},
		).WithHeader(
			&http.Header{
				utility.UserEmailHeaderKey:     []string{"proctor@example.com"},
				utility.AccessTokenHeaderKey:   []string{"access-token"},
				utility.ClientVersionHeaderKey: []string{version.ClientVersion},
				utility.ProcName:               []string{"run-sample"},
			},
		),
	)

	s.mockConfigLoader.On("Load").Return(proctorConfig, config.ConfigError{}).Once()

	response, err := s.testClient.ExecuteProc("run-sample", map[string]string{"SAMPLE_ARG1": "sample-value"})

	assert.Equal(t, "", response)
	assert.Equal(t, errors.New("Network Error!!!\nPost http://proctor.example.com/jobs/execute: Unknown Error"), err)
	s.mockConfigLoader.AssertExpectations(t)
}

func makeHostname(s string) string {
	return strings.TrimPrefix(s, "http://")
}

func (s *ClientTestSuite) TestLogStreamForAuthorizedUser() {
	t := s.T()
	logStreamAuthorizer := func(t *testing.T) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			upgrader := websocket.Upgrader{}
			assert.Equal(t, "proctor@example.com", r.Header.Get(utility.UserEmailHeaderKey))
			assert.Equal(t, "access-token", r.Header.Get(utility.AccessTokenHeaderKey))
			assert.Equal(t, version.ClientVersion, r.Header.Get(utility.ClientVersionHeaderKey))
			conn, _ := upgrader.Upgrade(w, r, nil)
			defer conn.Close()
		}
	}
	testServer := httptest.NewServer(logStreamAuthorizer(t))
	defer testServer.Close()
	proctorConfig := config.ProctorConfig{Host: makeHostname(testServer.URL), Email: "proctor@example.com", AccessToken: "access-token"}

	s.mockConfigLoader.On("Load").Return(proctorConfig, config.ConfigError{}).Once()

	err := s.testClient.StreamProcLogs("test-job-id")
	assert.NoError(t, err)
	s.mockConfigLoader.AssertExpectations(t)
}

func (s *ClientTestSuite) TestLogStreamForBadWebSocketHandshake() {
	t := s.T()
	badWebSocketHandshakeHandler := func() http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {}
	}
	testServer := httptest.NewServer(badWebSocketHandshakeHandler())
	defer testServer.Close()
	proctorConfig := config.ProctorConfig{Host: makeHostname(testServer.URL), Email: "proctor@example.com", AccessToken: "access-token"}

	s.mockConfigLoader.On("Load").Return(proctorConfig, config.ConfigError{}).Once()

	errStreamLogs := s.testClient.StreamProcLogs("test-job-id")
	assert.Equal(t, errors.New("websocket: bad handshake"), errStreamLogs)
	s.mockConfigLoader.AssertExpectations(t)
}

func (s *ClientTestSuite) TestLogStreamForUnauthorizedUser() {
	t := s.T()
	unauthorizedUserHandler := func() http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}
	testServer := httptest.NewServer(unauthorizedUserHandler())
	defer testServer.Close()
	proctorConfig := config.ProctorConfig{Host: makeHostname(testServer.URL), Email: "proctor@example.com", AccessToken: "access-token"}

	s.mockConfigLoader.On("Load").Return(proctorConfig, config.ConfigError{}).Once()

	errStreamLogs := s.testClient.StreamProcLogs("test-job-id")
	assert.Error(t, errors.New(http.StatusText(http.StatusUnauthorized)), errStreamLogs)
	s.mockConfigLoader.AssertExpectations(t)

}
