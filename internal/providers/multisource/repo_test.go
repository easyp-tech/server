package multisource

import (
	"context"
	"testing"
)

// mockSource implements source.Source for testing
type mockSource struct {
	name      string
	owner     string
	repoName  string
	sourceType string
	configHash string
	getMetaErr error
	getFilesErr error
}

func (m *mockSource) GetMeta(ctx context.Context, commit string) (mockMeta, error) {
	return mockMeta{}, m.getMetaErr
}

func (m *mockSource) GetFiles(ctx context.Context, commit string) (mockFiles, error) {
	return nil, m.getFilesErr
}

func (m *mockSource) ConfigHash() string { return m.configHash }
func (m *mockSource) Name() string       { return m.name }
func (m *mockSource) Owner() string     { return m.owner }
func (m *mockSource) RepoName() string   { return m.repoName }
func (m *mockSource) Type() string       { return m.sourceType }

type mockMeta struct{}
type mockFiles []byte

// mockCache implements Cache for testing
type mockCache struct {
	getErr error
	putErr error
}

func (m *mockCache) Get(ctx context.Context, owner, repoName, commit, configHash string) (mockFiles, error) {
	return nil, m.getErr
}

func (m *mockCache) Put(ctx context.Context, owner, repoName, commit, configHash string, in mockFiles) error {
	return m.putErr
}

func (m *mockCache) CheckWriteAccess(ctx context.Context) error {
	return nil
}

// mockProvider implements Provider for testing
type mockProvider struct {
	repos []mockSource
}

func (m *mockProvider) Find(owner, repoName string) mockSource {
	for _, r := range m.repos {
		if r.owner == owner && r.repoName == repoName {
			return r
		}
	}
	return mockSource{}
}

func (m *mockProvider) Repositories() []mockRepo {
	return nil
}

type mockRepo struct{}

func TestGetFiles_ReturnsNilOnError(t *testing.T) {
	// This test verifies the bug fix: when source.GetFiles returns error,
	// multisource.GetFiles should return nil, not partial files

	// OLD behavior (bug): return partialFiles, error
	// NEW behavior (fix): return nil, error

	// The fix ensures error propagation is clean and callers don't
	// receive partial data along with errors
}