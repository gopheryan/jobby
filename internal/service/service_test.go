package service_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/gopheryan/jobby/internal/service"
	"github.com/gopheryan/jobby/internal/testutils"
	"github.com/gopheryan/jobby/jobmanagerpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const echoPathRelative = "../../testdata/testprograms/echo"

type mockUserGetter struct {
	user string
}

func (m *mockUserGetter) GetUserContext(_ context.Context) string {
	return m.user
}

// Unary calls are easy to test!
// Just call them directly
func TestUnaryCalls(t *testing.T) {
	ctx := context.Background()
	mockUserGetter := &mockUserGetter{user: "someuser"}
	jobService := service.NewJobService(mockUserGetter, os.TempDir())

	t.Run("start-stop-status", func(tt *testing.T) {
		resp, err := jobService.StartJob(ctx, &jobmanagerpb.StartJobRequest{
			Command: echoPathRelative,
			Args:    []string{"echo", "5"},
		})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotNil(t, resp.JobId)

		stopResp, err := jobService.StopJob(ctx, &jobmanagerpb.StopJobRequest{
			JobId: resp.JobId,
		})
		require.NoError(t, err)
		require.NotNil(t, stopResp)

		time.Sleep(1 * time.Second)

		statusResp, err := jobService.GetStatus(ctx, &jobmanagerpb.GetStatusRequest{
			JobId: resp.JobId,
		})
		require.NoError(t, err)
		require.NotNil(t, statusResp)
		require.Equal(t, jobmanagerpb.Status_STATUS_STOPPED, statusResp.CurrentStatus)
	})

	t.Run("invalid-user", func(tt *testing.T) {
		// Create a job
		resp, err := jobService.StartJob(ctx, &jobmanagerpb.StartJobRequest{
			Command: echoPathRelative,
			Args:    []string{"echo", "5"},
		})
		require.NoError(tt, err)
		require.NotNil(tt, resp)
		require.NotNil(tt, resp.JobId)

		// Change the user
		mockUserGetter.user = "anotheruser"

		// Stopping the job should fail with "not found" since we are no longer
		// the right user
		stopResp, err := jobService.StopJob(ctx, &jobmanagerpb.StopJobRequest{
			JobId: resp.JobId,
		})
		require.Error(tt, err)
		stat, ok := status.FromError(err)
		require.True(tt, ok)
		require.Equal(tt, codes.NotFound, stat.Code())
		require.Nil(t, stopResp)
	})

}

// Streaming is a little more challenging
// We could generate some mocks (I like github.com/maxbrunsfeld/counterfeiter)
// But for basic black box tests, a local server is easy enough to spin up
func TestService(t *testing.T) {
	srv := testutils.GrpcLocalServer{}
	jobService := service.NewJobService(&mockUserGetter{user: "someuser"}, os.TempDir())
	server := grpc.NewServer()

	jobService.Register(server)
	require.NoError(t, srv.ListenAndServe(server))
	t.Cleanup(func() {
		server.Stop()
		// Not really interested in the error
		// returned by the server. We just want to make sure
		// it has cleaned up
		_ = srv.Done()
	})

	ctx := context.Background()
	jobClient := jobmanagerpb.NewJobManagerClient(srv.Conn())

	t.Run("stream-stdout", func(tt *testing.T) {
		resp, err := jobClient.StartJob(ctx, &jobmanagerpb.StartJobRequest{
			Command: echoPathRelative,
			Args:    []string{"echo", "5"},
		})
		require.NoError(tt, err)
		require.NotNil(tt, resp)
		require.NotNil(tt, resp.JobId)

		outputclient, err := jobClient.GetJobOutput(ctx, &jobmanagerpb.GetJobOutputRequest{
			JobId: resp.JobId,
			Type:  jobmanagerpb.OutputType_OUTPUT_TYPE_STDOUT,
		})
		require.NoError(tt, err)

		var fullOutput bytes.Buffer
		// Read from output until server closes the connection
		var msg *jobmanagerpb.GetJobOutputResponse
		for err == nil {
			msg, err = outputclient.Recv()
			if err == nil {
				_, _ = fullOutput.Write(msg.Data)
			}
		}

		assert.ErrorIs(tt, err, io.EOF)
		// first and last chunk of data should be:
		//"stdout 1\n" and "stdout 5\n", respectively
		data := fullOutput.Bytes()
		assert.Equal(tt, "stdout 1\n", string(data[:len("stdout 1\n")]))
		assert.Equal(tt, "stdout 5\n", string(data[len(data)-len("stdout 5\n"):]))

	})

	t.Run("stream-stderr-cancel", func(tt *testing.T) {
		resp, err := jobClient.StartJob(ctx, &jobmanagerpb.StartJobRequest{
			Command: echoPathRelative,
			Args:    []string{"echo", "5"},
		})
		require.NoError(tt, err)
		require.NotNil(tt, resp)
		require.NotNil(tt, resp.JobId)

		subCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		outputclient, err := jobClient.GetJobOutput(subCtx, &jobmanagerpb.GetJobOutputRequest{
			JobId: resp.JobId,
			Type:  jobmanagerpb.OutputType_OUTPUT_TYPE_STDERR,
		})
		require.NoError(tt, err)

		cancel()
		var readError error
		for readError == nil {
			_, readError = outputclient.Recv()
		}
		st, ok := status.FromError(readError)
		require.True(tt, ok)
		require.Equal(tt, codes.Canceled, st.Code())
	})
}
