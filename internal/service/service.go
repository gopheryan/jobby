package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"reflect"
	"sync"

	"github.com/google/uuid"
	"github.com/gopheryan/jobby/job"
	"github.com/gopheryan/jobby/jobmanagerpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const defaultOutputBufferSize = 4096

type UserGetter interface {
	GetUserContext(context.Context) string
}

type JobIdGetter interface {
	GetJobId() []byte
}

type jobData struct {
	// User who owns the job
	Owner string
	Job   *job.Job
}

type Jobby struct {
	jobmanagerpb.UnimplementedJobManagerServer
	// minimum buffer size use when reading from job output
	// Must be > 0. Internal reader will block until 'minimumRead'
	// bytes of data have been read from the job before sending
	// output to the caller
	minimumRead int
	// Used to determine which user a request is coming from
	// decouples the service from the auth strategy
	userGetter UserGetter
	// Base directory in which to store output files for jobs
	directory string
	// Keep track of jobs!
	// used as: map[uuid.UUID]*jobData
	jobDirectory sync.Map
}

func NewJobService(minRead int, userGetter UserGetter, dir string) *Jobby {
	return &Jobby{
		minimumRead: minRead,
		userGetter:  userGetter,
		directory:   dir,
	}
}

func (j *Jobby) Register(srv *grpc.Server) {
	srv.RegisterService(&jobmanagerpb.JobManager_ServiceDesc, j)
}

func (j *Jobby) GetJobOutput(req *jobmanagerpb.GetJobOutputRequest, srv jobmanagerpb.JobManager_GetJobOutputServer) error {
	subLogger := slog.With("user", j.userGetter.GetUserContext(srv.Context()), "request", req)
	subLogger.Info("Handling 'GetJobOutput' request")

	jobData, st := j.getJob(srv.Context(), req)
	if st != nil {
		return st.Err()
	}

	var reader io.ReadCloser
	var err error
	if req.Type == jobmanagerpb.OutputType_OUTPUT_TYPE_STDOUT {
		reader, err = jobData.Job.Stdout()
	} else if req.Type == jobmanagerpb.OutputType_OUTPUT_TYPE_STDERR {
		reader, err = jobData.Job.Stderr()
	} else {
		return status.Error(codes.InvalidArgument, "Must specify valid output type")
	}
	if err != nil {
		return status.Error(codes.Internal, "Error attaching to job output")
	}

	// The caller can cancel/detach at any time. This cancellation is communicated
	// to this handler via context cancellation
	context.AfterFunc(srv.Context(), func() {
		subLogger.Info("GetJobOutput request cancelled")
		// One call to 'Close' will shut down this whole operation...
		// This is going to cause an error on our reader
		if err = reader.Close(); err != nil {
			subLogger.Error("Error closing job output reader", slog.String("error", err.Error()))
		}

	})

	var readError error
	var sendError error
	var count int
	buf := make([]byte, defaultOutputBufferSize)
	// Read and send until one side fails
	for readError == nil && sendError == nil {
		count, readError = reader.Read(buf)
		if count > 0 {
			// Copy only as much as the reader returned
			dst := make([]byte, count)
			copy(dst, buf[:count])
			sendError = srv.Send(&jobmanagerpb.GetJobOutputResponse{
				Data: dst,
			})
		}
	}

	if readError != nil {
		if errors.Is(readError, io.EOF) || srv.Context().Err() != nil {
			// Silence readError if we got an EOF (clean end of stream)
			// or we notice that the context was cancelled
			// In the latter case, we intentionally closed our reader to
			// break out of the read
			readError = nil
		}
	}

	var allErrors error
	if allErrors = errors.Join(
		reader.Close(),
		sendError,
		readError,
	); allErrors != nil {
		// An actual error occurred
		subLogger.Error("Error occurred while reading process output", "error", allErrors)
		return status.Error(codes.Internal, "Error occurred while reading process output")
	} else {
		// gRPC library is smart enough to translate this
		// to the 'cancelled' status code for us (if it isn't nil)
		return srv.Context().Err()
	}
}

func jobStateToStatus(state job.State) *jobmanagerpb.Status {
	switch state {
	case job.JobStatusRunning:
		return jobmanagerpb.Status_STATUS_RUNNING.Enum()
	case job.JobStatusStopped:
		return jobmanagerpb.Status_STATUS_STOPPED.Enum()
	case job.JobstatusComplete:
		return jobmanagerpb.Status_STATUS_COMPLETE.Enum()
	default:
		return jobmanagerpb.Status_STATUS_UNSPECIFIED.Enum()
	}
}

func (j *Jobby) GetStatus(ctx context.Context, req *jobmanagerpb.GetStatusRequest) (*jobmanagerpb.GetStatusResponse, error) {
	slog.Info("Handling 'GetStatus' request", "user", j.userGetter.GetUserContext(ctx), "request", req)
	jobData, st := j.getJob(ctx, req)
	if st != nil {
		return nil, st.Err()
	}

	// In hindsight, I could've just used a non-pointer value in the protos
	// and returned '-1' when the exit code is not available.
	// You could also argue that nil/non-nil is a more explicit way
	// to communicate presence (which is where I lean)
	convertExitCode := func(i *int) *int32 {
		if i == nil {
			return nil
		}
		out := int32(*i)
		return &out
	}

	status := jobData.Job.Status()
	return &jobmanagerpb.GetStatusResponse{
		CurrentStatus: *jobStateToStatus(status.CurrentState),
		ExitCode:      convertExitCode(status.ReturnCode),
	}, nil
}

func (j *Jobby) StartJob(ctx context.Context, req *jobmanagerpb.StartJobRequest) (*jobmanagerpb.StartJobResponse, error) {
	subLogger := slog.With("user", j.userGetter.GetUserContext(ctx), "request", req)
	subLogger.Info("Handling 'StartJob' request")
	if req.Command == "" {
		return nil, status.Error(codes.InvalidArgument, "Must provide non-empty command")
	}

	jobId := uuid.New()
	newJob, err := job.New(job.JobArgs{
		Command:    req.Command,
		Args:       req.Args,
		StdoutPath: outFilePath(j.directory, jobId, "stdout"),
		StderrPath: outFilePath(j.directory, jobId, "sterr"),
	})
	if err != nil {
		// Don't leak error details to the caller
		// log them, but don't return them
		// (though, the client is ours so maybe it's ok?)
		subLogger.Error("Error starting job", "error", err)
		return nil, status.Error(codes.Internal, "Error starting job")
	}

	j.jobDirectory.Store(jobId, &jobData{
		Job:   newJob,
		Owner: j.userGetter.GetUserContext(ctx),
	})

	return &jobmanagerpb.StartJobResponse{
		JobId: jobId[:],
	}, nil
}

func (j *Jobby) StopJob(ctx context.Context, req *jobmanagerpb.StopJobRequest) (*jobmanagerpb.StopJobResponse, error) {
	sublogger := slog.With("user", j.userGetter.GetUserContext(ctx), "request", req)
	sublogger.Info("Handling 'StopJob' request")
	jobData, st := j.getJob(ctx, req)
	if st != nil {
		return nil, st.Err()
	}

	err := jobData.Job.Stop()
	if err != nil {
		sublogger.Error("Error stopping job", "error", err)
		return nil, status.Error(codes.Internal, fmt.Errorf("failed to stop job: %w", err).Error())
	} else {
		// For consistency, return non-nil responses when err == nil
		// If users become accustomed to us returning (nil, nil) on success
		// Then we're going to have a bad time when we want to start populating
		// this response message
		return &jobmanagerpb.StopJobResponse{}, nil
	}
}

// Try to make loading from the map a little less painful
func loadJob(m *sync.Map, id uuid.UUID) (*jobData, bool) {
	if data, exists := m.Load(id); exists {
		if job, ok := data.(*jobData); ok {
			return job, ok
		}
		slog.Error("Found invalid data type in map", "type", reflect.TypeOf(data), "job-id", id)
		return nil, false
	}
	return nil, false
}

// Most endpoints need to do this lookup so let's be consistent about it
func (j *Jobby) getJob(ctx context.Context, getter JobIdGetter) (*jobData, *status.Status) {
	jobId := getter.GetJobId()
	var id uuid.UUID
	var err error
	if id, err = uuid.FromBytes(jobId); err != nil {
		slog.Error("Failed to parse job id", "job-id", jobId, "error", err)
		return nil, status.New(codes.InvalidArgument, "Must provide valid job id")
	}

	if jobData, ok := loadJob(&j.jobDirectory, id); ok && jobData.Owner == j.userGetter.GetUserContext(ctx) {
		return jobData, nil
	} else {
		// Return the same "not found" error for cases where job is actually not found
		// or the user simply doesn't own the job. We could return "permission denied"
		// for the latter case, but maybe it's better not to communicate that this id
		// exists to a user that doesn't own it
		return nil, status.New(codes.NotFound, "No such job exists")
	}
}

func outFilePath(base string, u uuid.UUID, prefix string) string {
	return filepath.Join(base, fmt.Sprintf("%s-%s", u.String(), prefix))
}
