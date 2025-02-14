package commitqueue

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip/level"
	"github.com/stretchr/testify/suite"
)

type GitHubPRSenderSuite struct {
	suite.Suite
	q *CommitQueue
}

func TestGitHubPRSenderSuite(t *testing.T) {
	s := new(GitHubPRSenderSuite)
	suite.Run(t, s)
}

func (s *GitHubPRSenderSuite) SetupTest() {
	s.NoError(db.ClearCollections(Collection))
	cq := &CommitQueue{
		ProjectID: "mci",
		Queue: []CommitQueueItem{
			CommitQueueItem{
				Issue: "1",
			},
			CommitQueueItem{
				Issue: "2",
			},
		},
	}
	s.NoError(InsertQueue(cq))
}

func (s *GitHubPRSenderSuite) TestGithubPRLogger() {
	errLogger := &mockErrorLogger{}
	ghPRLogger, err := NewMockGithubPRLogger("mock_gh_pr_logger", errLogger)
	s.NoError(err)

	msg := GithubMergePR{
		Status:    evergreen.PatchSucceeded,
		ProjectID: "mci",
		Item:      "1",
		PRs: []event.PRInfo{
			{
				Owner:       "evergreen-ci",
				Repo:        "evergreen",
				Ref:         "deadbeef",
				CommitTitle: "PR (#1)",
				PRNum:       1,
			},
		},
	}
	c := NewGithubMergePRMessage(level.Info, msg)
	ghPRLogger.Send(c)
	s.Empty(errLogger.errList)
}

func (s *GitHubPRSenderSuite) TestDequeueFromCommitQueue() {
	s.NoError(dequeueFromCommitQueue("mci", "1"))
	cq, err := FindOneId("mci")
	s.NoError(err)
	s.Equal("2", cq.Next().Issue)
}
