package agent

import (
	"context"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/util"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/suite"
)

type StatusSuite struct {
	suite.Suite
	testOpts Options
	resp     statusResponse
	cancel   context.CancelFunc
}

func TestStatusSuite(t *testing.T) {
	suite.Run(t, new(StatusSuite))
}

func (s *StatusSuite) SetupTest() {
	s.testOpts = Options{
		HostID:     "none",
		StatusPort: 2286,
	}
	s.resp = buildResponse(s.testOpts)
}

func (s *StatusSuite) TearDownSuite() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *StatusSuite) TestBasicAssumptions() {
	s.Equal(s.resp.BuildId, evergreen.BuildRevision)
	s.Equal(s.resp.AgentPid, os.Getpid())
	s.Equal(s.resp.HostId, s.testOpts.HostID)
}

func (s *StatusSuite) TestPopulateSystemInfo() {
	grip.Alert(strings.Join(s.resp.SystemInfo.Errors, ";\n"))
	grip.Info(s.resp.SystemInfo)
	s.NotNil(s.resp.SystemInfo)
}

func (s *StatusSuite) TestProcessTreeInfo() {
	s.True(len(s.resp.ProcessTree) >= 1)
	for _, ps := range s.resp.ProcessTree {
		s.NotNil(ps)
	}
}

func (s *StatusSuite) TestAgentStartsStatusServer() {
	agt, err := New(s.testOpts, client.NewMock("url"))
	s.Require().NoError(err)

	mockCommunicator := agt.comm.(*client.Mock)
	mockCommunicator.NextTaskIsNil = true
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		_ = agt.Start(ctx)
	}()
	time.Sleep(100 * time.Millisecond)
	resp, err := http.Get("http://127.0.0.1:2286/status")
	s.Require().NoError(err)
	s.Equal(200, resp.StatusCode)
}

func (s *StatusSuite) TestAgentFailsToStartTwice() {
	resp, err := http.Get("http://127.0.0.1:2287/status")
	s.Error(err)

	s.testOpts.StatusPort = 2287
	agt, err := New(s.testOpts, client.NewMock("url"))
	s.Require().NoError(err)

	mockCommunicator := agt.comm.(*client.Mock)
	mockCommunicator.NextTaskIsNil = true
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	s.cancel = cancel

	first := make(chan error, 1)
	go func(c chan error) {
		c <- agt.Start(ctx)
	}(first)

	resp, err = http.Get("http://127.0.0.1:2287/status")
	if err != nil {
		// the service hasn't started.

		timer := time.NewTimer(0)
		defer timer.Stop()
	retryLoop:
		for {
			select {
			case <-ctx.Done():
				break retryLoop
			case <-timer.C:
				resp, err = http.Get("http://127.0.0.1:2287/status")
				if err == nil {
					break retryLoop
				}
				timer.Reset(10 * time.Millisecond)
			}
		}
	}

	s.Require().NoError(err)
	s.Equal(200, resp.StatusCode)

	second := make(chan error, 1)
	go func(c chan error) {
		secondCtx, secondCancel := context.WithCancel(context.Background())
		defer secondCancel()
		c <- agt.Start(secondCtx)
	}(second)

	err = <-second
	s.Error(err)
	s.Contains(err.Error(), "another agent is running on 2287")

	cancel()
	err = <-first
	s.Require().NoError(err)
}

func (s *StatusSuite) TestCheckOOMSucceeds() {
	if runtime.GOOS == "darwin" {
		s.T().Skip("OOM tests will not work on static mac hosts because logs are never cleared and will be too long to parse")
	}
	agt, err := New(s.testOpts, client.NewMock("url"))
	s.Require().NoError(err)
	mockCommunicator := agt.comm.(*client.Mock)
	mockCommunicator.NextTaskIsNil = true
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	go func() {
		_ = agt.Start(ctx)
	}()

	resp, err := http.Get("http://127.0.0.1:2286/jasper/v1/list/oom")
	if err != nil {
		// the service hasn't started.

		timer := time.NewTimer(0)
		defer timer.Stop()
	retryLoop:
		for {
			select {
			case <-ctx.Done():
				break retryLoop
			case <-timer.C:
				resp, err = http.Get("http://127.0.0.1:2286/jasper/v1/list/oom")
				if err == nil {
					break retryLoop
				}
				timer.Reset(10 * time.Millisecond)
			}
		}
	}

	s.Require().NoError(err)
	s.Equal(200, resp.StatusCode)

	tracker := jasper.NewOOMTracker()
	s.NoError(util.ReadJSONInto(resp.Body, tracker))
	wasOomKilled, pids := tracker.Report()
	s.False(wasOomKilled)
	s.Len(pids, 0)
}
