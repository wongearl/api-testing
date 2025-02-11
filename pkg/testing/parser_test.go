package testing

import (
	"io"
	"net/http"
	"testing"

	_ "embed"

	"github.com/linuxsuren/api-testing/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	suite, err := Parse("../../sample/testsuite-gitlab.yaml")
	if assert.Nil(t, err) && assert.NotNil(t, suite) {
		assert.Equal(t, "Gitlab", suite.Name)
		assert.Equal(t, 2, len(suite.Items))
		assert.Equal(t, TestCase{
			Name: "projects",
			Request: Request{
				API: "https://gitlab.com/api/v4/projects",
			},
			Expect: Response{
				StatusCode: http.StatusOK,
				Schema: `{
  "type": "array"
}
`,
			},
		}, suite.Items[0])
	}

	_, err = Parse("testdata/invalid-testcase.yaml")
	assert.NotNil(t, err)
}

func TestDuplicatedNames(t *testing.T) {
	_, err := Parse("testdata/duplicated-names.yaml")
	assert.NotNil(t, err)

	_, err = ParseFromData([]byte("fake"))
	assert.NotNil(t, err)
}

func TestRequestRender(t *testing.T) {
	tests := []struct {
		name    string
		request *Request
		verify  func(t *testing.T, req *Request)
		ctx     interface{}
		hasErr  bool
	}{{
		name: "slice as context",
		request: &Request{
			API:  "http://localhost/{{index . 0}}",
			Body: "{{index . 1}}",
		},
		ctx:    []string{"foo", "bar"},
		hasErr: false,
		verify: func(t *testing.T, req *Request) {
			assert.Equal(t, "http://localhost/foo", req.API)
			assert.Equal(t, "bar", req.Body)
		},
	}, {
		name:    "default values",
		request: &Request{},
		verify: func(t *testing.T, req *Request) {
			assert.Equal(t, http.MethodGet, req.Method)
		},
		hasErr: false,
	}, {
		name:    "context is nil",
		request: &Request{},
		ctx:     nil,
		hasErr:  false,
	}, {
		name: "body from file",
		request: &Request{
			BodyFromFile: "testdata/generic_body.json",
		},
		ctx: TestCase{
			Name: "linuxsuren",
		},
		hasErr: false,
		verify: func(t *testing.T, req *Request) {
			assert.Equal(t, `{"name": "linuxsuren"}`, req.Body)
		},
	}, {
		name: "body file not found",
		request: &Request{
			BodyFromFile: "testdata/fake",
		},
		hasErr: true,
	}, {
		name: "invalid API as template",
		request: &Request{
			API: "{{.name}",
		},
		hasErr: true,
	}, {
		name: "failed with API render",
		request: &Request{
			API: "{{.name}}",
		},
		ctx:    TestCase{},
		hasErr: true,
	}, {
		name: "invalid body as template",
		request: &Request{
			Body: "{{.name}",
		},
		hasErr: true,
	}, {
		name: "failed with body render",
		request: &Request{
			Body: "{{.name}}",
		},
		ctx:    TestCase{},
		hasErr: true,
	}, {
		name: "form render",
		request: &Request{
			Form: map[string]string{
				"key": "{{.Name}}",
			},
		},
		ctx: TestCase{Name: "linuxsuren"},
		verify: func(t *testing.T, req *Request) {
			assert.Equal(t, "linuxsuren", req.Form["key"])
		},
		hasErr: false,
	}, {
		name: "header render",
		request: &Request{
			Header: map[string]string{
				"key": "{{.Name}}",
			},
		},
		ctx: TestCase{Name: "linuxsuren"},
		verify: func(t *testing.T, req *Request) {
			assert.Equal(t, "linuxsuren", req.Header["key"])
		},
		hasErr: false,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Render(tt.ctx)
			if assert.Equal(t, tt.hasErr, err != nil, err) && tt.verify != nil {
				tt.verify(t, tt.request)
			}
		})
	}
}

func TestResponseRender(t *testing.T) {
	tests := []struct {
		name     string
		response *Response
		verify   func(t *testing.T, req *Response)
		ctx      interface{}
		hasErr   bool
	}{{
		name:     "blank response",
		response: &Response{},
		verify: func(t *testing.T, req *Response) {
			assert.Equal(t, http.StatusOK, req.StatusCode)
		},
		hasErr: false,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.response.Render(tt.ctx)
			if assert.Equal(t, tt.hasErr, err != nil, err) && tt.verify != nil {
				tt.verify(t, tt.response)
			}
		})
	}
}

func TestEmptyThenDefault(t *testing.T) {
	tests := []struct {
		name   string
		val    string
		defVal string
		expect string
	}{{
		name:   "empty string",
		val:    "",
		defVal: "abc",
		expect: "abc",
	}, {
		name:   "blank string",
		val:    " ",
		defVal: "abc",
		expect: "abc",
	}, {
		name:   "not empty or blank string",
		val:    "abc",
		defVal: "def",
		expect: "abc",
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := emptyThenDefault(tt.val, tt.defVal)
			assert.Equal(t, tt.expect, result, result)
		})
	}

	assert.Equal(t, 1, zeroThenDefault(0, 1))
	assert.Equal(t, 1, zeroThenDefault(1, 2))
}

func TestTestCase(t *testing.T) {
	testCase, err := ParseTestCaseFromData([]byte(testCaseContent))
	assert.Nil(t, err)
	assert.Equal(t, &TestCase{
		Name: "projects",
		Request: Request{
			API: "https://foo",
		},
		Expect: Response{
			StatusCode: http.StatusOK,
		},
	}, testCase)
}

func TestGetBody(t *testing.T) {
	defaultBody := "fake body"

	tests := []struct {
		name        string
		req         *Request
		expectBody  string
		containBody string
		expectErr   bool
	}{{
		name:       "normal body",
		req:        &Request{Body: defaultBody},
		expectBody: defaultBody,
	}, {
		name:       "body from file",
		req:        &Request{BodyFromFile: "testdata/testcase.yaml"},
		expectBody: testCaseContent,
	}, {
		name: "multipart form data",
		req: &Request{
			Header: map[string]string{
				util.ContentType: util.MultiPartFormData,
			},
			Form: map[string]string{
				"key": "value",
			},
		},
		containBody: "name=\"key\"\r\n\r\nvalue\r\n",
	}, {
		name: "normal form",
		req: &Request{
			Header: map[string]string{
				util.ContentType: util.Form,
			},
			Form: map[string]string{
				"name": "linuxsuren",
			},
		},
		expectBody: "name=linuxsuren",
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, err := tt.req.GetBody()
			if tt.expectErr {
				assert.NotNil(t, err)
			} else {
				assert.NotNil(t, reader)
				data, err := io.ReadAll(reader)
				assert.Nil(t, err)
				if tt.expectBody != "" {
					assert.Equal(t, tt.expectBody, string(data))
				} else {
					assert.Contains(t, string(data), tt.containBody)
				}
			}
		})
	}
}

//go:embed testdata/testcase.yaml
var testCaseContent string
