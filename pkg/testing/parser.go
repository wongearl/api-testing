package testing

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/linuxsuren/api-testing/pkg/render"
	"github.com/linuxsuren/api-testing/pkg/util"
	"github.com/linuxsuren/api-testing/sample"
	"github.com/xeipuuv/gojsonschema"
)

// Parse parses a file and returns the test suite
func Parse(configFile string) (testSuite *TestSuite, err error) {
	var data []byte
	if data, err = os.ReadFile(configFile); err == nil {
		testSuite, err = ParseFromData(data)
	}

	// schema validation
	if err == nil {
		// convert YAML to JSON
		var jsonData []byte
		if jsonData, err = yaml.YAMLToJSON(data); err == nil {
			schemaLoader := gojsonschema.NewStringLoader(sample.Schema)
			documentLoader := gojsonschema.NewBytesLoader(jsonData)

			var result *gojsonschema.Result
			if result, err = gojsonschema.Validate(schemaLoader, documentLoader); err == nil {
				if !result.Valid() {
					err = fmt.Errorf("%v", result.Errors())
				}
			}
		}
	}
	return
}

// ParseFromData parses data and returns the test suite
func ParseFromData(data []byte) (testSuite *TestSuite, err error) {
	testSuite = &TestSuite{}
	if err = yaml.Unmarshal(data, testSuite); err != nil {
		return
	}

	names := map[string]struct{}{}
	for _, item := range testSuite.Items {
		if _, ok := names[item.Name]; !ok {
			names[item.Name] = struct{}{}
		} else {
			err = fmt.Errorf("having duplicated name '%s'", item.Name)
			break
		}
	}
	return
}

// ParseTestCaseFromData parses the data to a test case
func ParseTestCaseFromData(data []byte) (testCase *TestCase, err error) {
	testCase = &TestCase{}
	err = yaml.Unmarshal(data, testCase)
	return
}

// Render injects the template based context
func (r *Request) Render(ctx interface{}) (err error) {
	// template the API
	var result string
	if result, err = render.Render("api", r.API, ctx); err == nil {
		r.API = result
	} else {
		err = fmt.Errorf("failed render '%s', %v", r.API, err)
		return
	}

	// read body from file
	if r.BodyFromFile != "" {
		var data []byte
		if data, err = os.ReadFile(r.BodyFromFile); err != nil {
			return
		}
		r.Body = strings.TrimSpace(string(data))
	}

	// template the header
	for key, val := range r.Header {
		if result, err = render.Render("header", val, ctx); err == nil {
			r.Header[key] = result
		} else {
			return
		}
	}

	// template the body
	if result, err = render.Render("body", r.Body, ctx); err == nil {
		r.Body = result
	} else {
		return
	}

	// template the form
	for key, val := range r.Form {
		if result, err = render.Render("form", val, ctx); err == nil {
			r.Form[key] = result
		} else {
			return
		}
	}

	// setting default values
	r.Method = emptyThenDefault(r.Method, http.MethodGet)
	return
}

// GetBody returns the request body
func (r *Request) GetBody() (reader io.Reader, err error) {
	if len(r.Form) > 0 {
		if r.Header[util.ContentType] == util.MultiPartFormData {
			multiBody := &bytes.Buffer{}
			writer := multipart.NewWriter(multiBody)
			for key, val := range r.Form {
				writer.WriteField(key, val)
			}

			_ = writer.Close()
			reader = multiBody
			r.Header[util.ContentType] = writer.FormDataContentType()
		} else if r.Header[util.ContentType] == util.Form {
			data := url.Values{}
			for key, val := range r.Form {
				data.Set(key, val)
			}
			reader = strings.NewReader(data.Encode())
		}
	} else if r.Body != "" {
		reader = bytes.NewBufferString(r.Body)
	} else if r.BodyFromFile != "" {
		var data []byte
		if data, err = os.ReadFile(r.BodyFromFile); err == nil {
			reader = bytes.NewBufferString(string(data))
		}
	}
	return
}

// Render renders the response
func (r *Response) Render(ctx interface{}) (err error) {
	r.StatusCode = zeroThenDefault(r.StatusCode, http.StatusOK)
	return
}

func zeroThenDefault(val, defVal int) int {
	if val == 0 {
		val = defVal
	}
	return val
}

func emptyThenDefault(val, defVal string) string {
	if strings.TrimSpace(val) == "" {
		val = defVal
	}
	return val
}
