package cmd

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/linuxsuren/api-testing/pkg/limit"
	"github.com/linuxsuren/api-testing/pkg/render"
	"github.com/linuxsuren/api-testing/pkg/runner"
	"github.com/linuxsuren/api-testing/pkg/testing"
	"github.com/spf13/cobra"
	"golang.org/x/sync/semaphore"
)

type runOption struct {
	pattern            string
	duration           time.Duration
	requestTimeout     time.Duration
	requestIgnoreError bool
	thread             int64
	context            context.Context
	qps                int32
	burst              int32
	limiter            limit.RateLimiter
	startTime          time.Time
	reporter           runner.TestReporter
	reportWriter       runner.ReportResultWriter
	report             string
}

func newDefaultRunOption() *runOption {
	return &runOption{
		reporter:     runner.NewmemoryTestReporter(),
		reportWriter: runner.NewResultWriter(os.Stdout),
	}
}

func newDiskCardRunOption() *runOption {
	return &runOption{
		reporter:     runner.NewDiscardTestReporter(),
		reportWriter: runner.NewDiscardResultWriter(),
	}
}

// CreateRunCommand returns the run command
func CreateRunCommand() (cmd *cobra.Command) {
	opt := newDefaultRunOption()
	cmd = &cobra.Command{
		Use:     "run",
		Aliases: []string{"r"},
		Example: `atest run -p sample.yaml
See also https://github.com/LinuxSuRen/api-testing/tree/master/sample`,
		Short:   "Run the test suite",
		PreRunE: opt.preRunE,
		RunE:    opt.runE,
	}

	// set flags
	flags := cmd.Flags()
	flags.StringVarP(&opt.pattern, "pattern", "p", "test-suite-*.yaml",
		"The file pattern which try to execute the test cases")
	flags.DurationVarP(&opt.duration, "duration", "", 0, "Running duration")
	flags.DurationVarP(&opt.requestTimeout, "request-timeout", "", time.Minute, "Timeout for per request")
	flags.BoolVarP(&opt.requestIgnoreError, "request-ignore-error", "", false, "Indicate if ignore the request error")
	flags.Int64VarP(&opt.thread, "thread", "", 1, "Threads of the execution")
	flags.Int32VarP(&opt.qps, "qps", "", 5, "QPS")
	flags.Int32VarP(&opt.burst, "burst", "", 5, "burst")
	flags.StringVarP(&opt.report, "report", "", "", "The type of target report")
	return
}

func (o *runOption) preRunE(cmd *cobra.Command, args []string) (err error) {
	switch o.report {
	case "markdown", "md":
		o.reportWriter = runner.NewMarkdownResultWriter(cmd.OutOrStdout())
	}
	return
}

func (o *runOption) runE(cmd *cobra.Command, args []string) (err error) {
	var files []string
	o.startTime = time.Now()
	o.context = cmd.Context()
	o.limiter = limit.NewDefaultRateLimiter(o.qps, o.burst)
	defer func() {
		cmd.Printf("consume: %s\n", time.Now().Sub(o.startTime).String())
		o.limiter.Stop()
	}()

	if files, err = filepath.Glob(o.pattern); err == nil {
		for i := range files {
			item := files[i]
			if err = o.runSuiteWithDuration(item); err != nil {
				return
			}
		}
	}

	// print the report
	if err == nil {
		var results []runner.ReportResult
		if results, err = o.reporter.ExportAllReportResults(); err == nil {
			err = o.reportWriter.Output(results)
		}
	}
	return
}

func (o *runOption) runSuiteWithDuration(suite string) (err error) {
	sem := semaphore.NewWeighted(o.thread)
	stop := false
	var timeout *time.Ticker
	if o.duration > 0 {
		timeout = time.NewTicker(o.duration)
	} else {
		// make sure having a valid timer
		timeout = time.NewTicker(time.Second)
	}
	errChannel := make(chan error, 10*o.thread)
	stopSingal := make(chan struct{}, 1)
	var wait sync.WaitGroup

	for !stop {
		select {
		case <-timeout.C:
			stop = true
			stopSingal <- struct{}{}
		case err = <-errChannel:
			if err != nil {
				stop = true
			}
		default:
			if err := sem.Acquire(o.context, 1); err != nil {
				continue
			}
			wait.Add(1)

			go func(ch chan error, sem *semaphore.Weighted) {
				now := time.Now()
				defer sem.Release(1)
				defer wait.Done()
				defer func() {
					fmt.Println("routing end with", time.Now().Sub(now))
				}()

				dataContext := getDefaultContext()
				ch <- o.runSuite(suite, dataContext, o.context, stopSingal)
			}(errChannel, sem)
			if o.duration <= 0 {
				stop = true
			}
		}
	}

	select {
	case err = <-errChannel:
	case <-stopSingal:
	}

	wait.Wait()
	return
}

func (o *runOption) runSuite(suite string, dataContext map[string]interface{}, ctx context.Context, stopSingal chan struct{}) (err error) {
	var testSuite *testing.TestSuite
	if testSuite, err = testing.Parse(suite); err != nil {
		return
	}

	var result string
	if result, err = render.Render("base api", testSuite.API, dataContext); err == nil {
		testSuite.API = result
		testSuite.API = strings.TrimSuffix(testSuite.API, "/")
	} else {
		return
	}

	for _, testCase := range testSuite.Items {
		// reuse the API prefix
		if strings.HasPrefix(testCase.Request.API, "/") {
			testCase.Request.API = fmt.Sprintf("%s%s", testSuite.API, testCase.Request.API)
		}

		var output interface{}
		select {
		case <-stopSingal:
			return
		default:
			// reuse the API prefix
			if strings.HasPrefix(testCase.Request.API, "/") {
				testCase.Request.API = fmt.Sprintf("%s%s", testSuite.API, testCase.Request.API)
			}

			setRelativeDir(suite, &testCase)
			o.limiter.Accept()

			ctxWithTimeout, _ := context.WithTimeout(ctx, o.requestTimeout)

			simpleRunner := runner.NewSimpleTestCaseRunner()
			simpleRunner.WithTestReporter(o.reporter)
			if output, err = simpleRunner.RunTestCase(&testCase, dataContext, ctxWithTimeout); err != nil && !o.requestIgnoreError {
				return
			}
		}
		dataContext[testCase.Name] = output
	}
	return
}

func getDefaultContext() map[string]interface{} {
	return map[string]interface{}{}
}

func setRelativeDir(configFile string, testcase *testing.TestCase) {
	dir := filepath.Dir(configFile)

	for i := range testcase.Prepare.Kubernetes {
		testcase.Prepare.Kubernetes[i] = path.Join(dir, testcase.Prepare.Kubernetes[i])
	}
}
