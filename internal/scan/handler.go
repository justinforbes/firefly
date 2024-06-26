package scan

import (
	"fmt"

	"github.com/Brum3ns/firefly/internal/config"
	"github.com/Brum3ns/firefly/internal/knowledge"
	"github.com/Brum3ns/firefly/internal/output"
	"github.com/Brum3ns/firefly/pkg/request"
	"github.com/Brum3ns/firefly/pkg/waitgroup"
)

type Handler struct {
	Process   scan
	WaitGroup waitgroup.WaitGroup
	JobQueue  chan Job
	Pool      chan chan Job
	quit      chan bool
	Config
}

// Config given by user input to adapt the scanning process
type Config struct {
	Threads       int
	PayloadVerify string

	// The scanner contains the points to a base structure that contains the base structure
	// of all the scanner techniques the handler need. This save memory and gain better preformence in the overall preformance.
	// Note : (Static data stored. Read struct DESC)
	Scanner *config.Scanner

	// This map holds all the knowledge of all the targets
	// !Note : (This map *MUST* be static and not modifed)
	Knowledge map[string]knowledge.Knowledge
}

type Job struct {
	OK_knowledge bool
	Knowledge    knowledge.Knowledge
	Encode       []string
	Http         request.Result
}

// Note : (Alias of structure "output.ResultFinal")
type Result struct {
	Output output.ResultFinal
	Error  error
}

// Start the handler for the workers by giving the tasks to preform and the amount of workers.
func NewHandler(config Config) Handler {
	return Handler{
		Config:   config,
		JobQueue: make(chan Job),
		Pool:     make(chan chan Job, config.Threads),
	}
}

// Start all the processes and assign tasks (jobs) to the scanners that are listening. Use the method "Stop()" to stop the scanner.
// Note : (The scanner handler *MUST* run inside a [go]rutine. It can only stop from the method "Stop()" that do send a stop signal to the handler)
func (e *Handler) Run(listener chan<- Result) {
	var pResult = make(chan scanResult)

	//Validate process amount:
	if e.Threads <= 0 {
		e.Threads = 1
	}

	// Start the amount of processes related to the amount of given threads:
	for i := 0; i < e.Threads; i++ {
		e.Process = newScan(e.Config.Scanner, e.Pool)
		e.Process.spawnScan(pResult)
	}

	// Listen for new jobs from the queue and send it to the job channel for the workers to handle it:
	go func() {
		for {
			select {
			case job := <-e.JobQueue:
				go func(job Job) {
					//Get an available job channel from any running process:
					jobChannel := <-e.Pool

					//Give the available process the job:
					jobChannel <- job
				}(job)

				//Listen for result from any process, if a result is recived, then send it to the listener [chan]nel:
			case r := <-pResult:
				listener <- makeResult(r)
				e.WaitGroup.Done()
			}
		}
	}()

	// Listen a stop signal then wait until all background processes are completed:
	if <-e.quit {
		e.WaitGroup.Wait()
		fmt.Println(":: Scanner handler stopped")
		return
	}
}

// Add new jobs (tasks) to be performed by the handler processes:
func (e *Handler) AddJob(httpResult request.Result) {
	// Get knowledge for the specific target
	knowledge, ok := e.GetKnowledge(httpResult.TargetHashId)

	e.WaitGroup.Add(1)
	e.JobQueue <- Job{
		Http:         httpResult,
		Knowledge:    knowledge,
		OK_knowledge: ok,
	}
}

func (e *Handler) GetKnowledge(hashid string) (knowledge.Knowledge, bool) {
	knowledge, ok := e.Knowledge[hashid]
	return knowledge, ok
}

// Get the amount of job that are active
func (e *Handler) GetJobInProcess() int {
	return e.WaitGroup.GetCount()
}

// Wait until all jobs are done
func (e *Handler) Wait() {
	e.WaitGroup.Wait()
}

func (e *Handler) Stop() {
	e.quit <- true
}

// Start the extract scanning process
func makeResult(pResult scanResult) Result {
	req := pResult.Http.Request
	resp := pResult.Http.Response

	return Result{
		Output: output.ResultFinal{
			TargetHashId:   pResult.Http.TargetHashId,
			RequestId:      pResult.Http.RequestId,
			Tag:            pResult.Http.Tag,
			Date:           pResult.Http.Date,
			Payload:        pResult.Http.Payload,
			UnkownBehavior: pResult.UnkownBehavior,
			OK:             true,

			Request: output.Request{
				URL:         req.RequestURI,
				URLOriginal: req.URLOriginal,
				Host:        req.URL.Host,
				Scheme:      req.URL.Scheme,
				Method:      req.Method,
				PostBody:    req.Body,
				Proto:       req.Proto,
				Headers:     req.HeadersOriginal,
			},
			Response: output.Response{
				Time:          resp.Time,
				Host:          resp.Request.Host,
				Body:          resp.Body,
				Title:         resp.Title,
				Proto:         resp.Proto,
				IPAddress:     resp.IPAddress,
				StatusCode:    resp.StatusCode,
				WordCount:     resp.WordCount,
				LineCount:     resp.LineCount,
				ContentType:   resp.ContentType,
				ContentLength: resp.ResponseBodySize,
				HeaderAmount:  resp.HeaderAmount,
				Headers:       resp.Header,
			},
			Scanner: output.Scanner{
				Extract:        pResult.Extract,
				Diff:           pResult.Difference,
				Transformation: pResult.Transformation,
				//Data...
			},

			Error: nil,
		},
		Error: nil,
	}
}
