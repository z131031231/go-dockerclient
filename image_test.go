// Copyright 2013 go-dockerclient authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package docker

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func newTestClient(rt http.RoundTripper) Client {
	endpoint := "http://localhost:4243"
	u, _ := parseEndpoint("http://localhost:4243", false)
	testAPIVersion, _ := NewAPIVersion("1.17")
	client := Client{
		HTTPClient:             &http.Client{Transport: rt},
		Dialer:                 &net.Dialer{},
		endpoint:               endpoint,
		endpointURL:            u,
		SkipServerVersionCheck: true,
		serverAPIVersion:       testAPIVersion,
	}
	return client
}

type stdoutMock struct {
	*bytes.Buffer
}

func (m stdoutMock) Close() error {
	return nil
}

type stdinMock struct {
	*bytes.Buffer
}

func (m stdinMock) Close() error {
	return nil
}

func TestListImages(t *testing.T) {
	t.Parallel()
	body := `[
     {
             "Repository":"base",
             "Tag":"ubuntu-12.10",
             "Id":"b750fe79269d",
             "Created":1364102658
     },
     {
             "Repository":"base",
             "Tag":"ubuntu-quantal",
             "Id":"b750fe79269d",
             "Created":1364102658
     },
     {
             "RepoTag": [
             "ubuntu:12.04",
             "ubuntu:precise",
             "ubuntu:latest"
             ],
             "Id": "8dbd9e392a964c",
             "Created": 1365714795,
             "Size": 131506275,
             "VirtualSize": 131506275
      },
      {
             "RepoTag": [
             "ubuntu:12.10",
             "ubuntu:quantal"
             ],
             "ParentId": "27cf784147099545",
             "Id": "b750fe79269d2e",
             "Created": 1364102658,
             "Size": 24653,
             "VirtualSize": 180116135
      }
]`
	var expected []APIImages
	err := json.Unmarshal([]byte(body), &expected)
	if err != nil {
		t.Fatal(err)
	}
	client := newTestClient(&FakeRoundTripper{message: body, status: http.StatusOK})
	images, err := client.ListImages(ListImagesOptions{})
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(images, expected) {
		t.Errorf("ListImages: Wrong return value. Want %#v. Got %#v.", expected, images)
	}
}

func TestListImagesParameters(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "null", status: http.StatusOK}
	client := newTestClient(fakeRT)
	_, err := client.ListImages(ListImagesOptions{All: false})
	if err != nil {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	if req.Method != http.MethodGet {
		t.Errorf("ListImages({All: false}: Wrong HTTP method. Want GET. Got %s.", req.Method)
	}
	if all := req.URL.Query().Get("all"); all != "0" && all != "" {
		t.Errorf("ListImages({All: false}): Wrong parameter. Want all=0 or not present at all. Got all=%s", all)
	}
	fakeRT.Reset()
	_, err = client.ListImages(ListImagesOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	req = fakeRT.requests[0]
	if all := req.URL.Query().Get("all"); all != "1" {
		t.Errorf("ListImages({All: true}): Wrong parameter. Want all=1. Got all=%s", all)
	}
	fakeRT.Reset()
	_, err = client.ListImages(ListImagesOptions{Filters: map[string][]string{
		"dangling": {"true"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	req = fakeRT.requests[0]
	body := req.URL.Query().Get("filters")
	var filters map[string][]string
	err = json.Unmarshal([]byte(body), &filters)
	if err != nil {
		t.Fatal(err)
	}
	if len(filters["dangling"]) != 1 || filters["dangling"][0] != "true" {
		t.Errorf("ListImages(dangling=[true]): Wrong filter map. Want dangling=[true], got dangling=%v", filters["dangling"])
	}
}

func TestImageHistory(t *testing.T) {
	t.Parallel()
	body := `[
	{
		"Id": "25daec02219d2d852f7526137213a9b199926b4b24e732eab5b8bc6c49bd470e",
		"Tags": [
			"debian:7.6",
			"debian:latest",
			"debian:7",
			"debian:wheezy"
		],
		"Created": 1409856216,
		"CreatedBy": "/bin/sh -c #(nop) CMD [/bin/bash]"
	},
	{
		"Id": "41026a5347fb5be6ed16115bf22df8569697139f246186de9ae8d4f67c335dce",
		"Created": 1409856213,
		"CreatedBy": "/bin/sh -c #(nop) ADD file:1ee9e97209d00e3416a4543b23574cc7259684741a46bbcbc755909b8a053a38 in /",
		"Size": 85178663
	},
	{
		"Id": "511136ea3c5a64f264b78b5433614aec563103b4d4702f3ba7d4d2698e22c158",
		"Tags": [
			"scratch:latest"
		],
		"Created": 1371157430
	}
]`
	var expected []ImageHistory
	err := json.Unmarshal([]byte(body), &expected)
	if err != nil {
		t.Fatal(err)
	}
	client := newTestClient(&FakeRoundTripper{message: body, status: http.StatusOK})
	history, err := client.ImageHistory("debian:latest")
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(history, expected) {
		t.Errorf("ImageHistory: Wrong return value. Want %#v. Got %#v.", expected, history)
	}
}

func TestRemoveImage(t *testing.T) {
	t.Parallel()
	name := "test"
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusNoContent}
	client := newTestClient(fakeRT)
	err := client.RemoveImage(name)
	if err != nil {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	expectedMethod := http.MethodDelete
	if req.Method != expectedMethod {
		t.Errorf("RemoveImage(%q): Wrong HTTP method. Want %s. Got %s.", name, expectedMethod, req.Method)
	}
	u, _ := url.Parse(client.getURL("/images/" + name))
	if req.URL.Path != u.Path {
		t.Errorf("RemoveImage(%q): Wrong request path. Want %q. Got %q.", name, u.Path, req.URL.Path)
	}
}

func TestRemoveImageNotFound(t *testing.T) {
	t.Parallel()
	client := newTestClient(&FakeRoundTripper{message: "no such image", status: http.StatusNotFound})
	err := client.RemoveImage("test:")
	if !errors.Is(err, ErrNoSuchImage) {
		t.Errorf("RemoveImage: wrong error. Want %#v. Got %#v.", ErrNoSuchImage, err)
	}
}

func TestRemoveImageExtended(t *testing.T) {
	t.Parallel()
	name := "test"
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusNoContent}
	client := newTestClient(fakeRT)
	err := client.RemoveImageExtended(name, RemoveImageOptions{Force: true, NoPrune: true})
	if err != nil {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	expectedMethod := http.MethodDelete
	if req.Method != expectedMethod {
		t.Errorf("RemoveImage(%q): Wrong HTTP method. Want %s. Got %s.", name, expectedMethod, req.Method)
	}
	u, _ := url.Parse(client.getURL("/images/" + name))
	if req.URL.Path != u.Path {
		t.Errorf("RemoveImage(%q): Wrong request path. Want %q. Got %q.", name, u.Path, req.URL.Path)
	}
	expectedQuery := "force=1&noprune=1"
	if query := req.URL.Query().Encode(); query != expectedQuery {
		t.Errorf("PushImage: Wrong query string. Want %q. Got %q.", expectedQuery, query)
	}
}

func TestInspectImage(t *testing.T) {
	t.Parallel()
	body := `{
     "Id":"b750fe79269d2ec9a3c593ef05b4332b1d1a02a62b4accb2c21d589ff2f5f2dc",
     "Parent":"27cf784147099545",
     "Created":"2013-03-23T22:24:18.818426Z",
     "Container":"3d67245a8d72ecf13f33dffac9f79dcdf70f75acb84d308770391510e0c23ad0",
     "ContainerConfig":{"Memory":1},
     "VirtualSize":12345,
     "RootFS": {
       "Type": "layers",
       "Layers": [
         "sha256:05a0deb2e405eb3095ab646dc1695a26bffe8bd4071e3af90efcf16e9d3f6d93",
         "sha256:4c5db681b9aa9ab1cf666ec969a810c8ff4410e70e06394670dc4f3bf595532f"
       ]
    }
}`

	created, err := time.Parse(time.RFC3339Nano, "2013-03-23T22:24:18.818426Z")
	if err != nil {
		t.Fatal(err)
	}

	expected := Image{
		ID:        "b750fe79269d2ec9a3c593ef05b4332b1d1a02a62b4accb2c21d589ff2f5f2dc",
		Parent:    "27cf784147099545",
		Created:   created,
		Container: "3d67245a8d72ecf13f33dffac9f79dcdf70f75acb84d308770391510e0c23ad0",
		ContainerConfig: Config{
			Memory: 1,
		},
		VirtualSize: 12345,
		RootFS: &RootFS{
			Type: "layers",
			Layers: []string{
				"sha256:05a0deb2e405eb3095ab646dc1695a26bffe8bd4071e3af90efcf16e9d3f6d93",
				"sha256:4c5db681b9aa9ab1cf666ec969a810c8ff4410e70e06394670dc4f3bf595532f",
			},
		},
	}
	fakeRT := &FakeRoundTripper{message: body, status: http.StatusOK}
	client := newTestClient(fakeRT)
	image, err := client.InspectImage(expected.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*image, expected) {
		t.Errorf("InspectImage(%q): Wrong image returned. Want %#v. Got %#v.", expected.ID, expected, *image)
	}
	req := fakeRT.requests[0]
	if req.Method != http.MethodGet {
		t.Errorf("InspectImage(%q): Wrong HTTP method. Want GET. Got %s.", expected.ID, req.Method)
	}
	u, _ := url.Parse(client.getURL("/images/" + expected.ID + "/json"))
	if req.URL.Path != u.Path {
		t.Errorf("InspectImage(%q): Wrong request URL. Want %q. Got %q.", expected.ID, u.Path, req.URL.Path)
	}
}

func TestInspectImageNotFound(t *testing.T) {
	t.Parallel()
	client := newTestClient(&FakeRoundTripper{message: "no such image", status: http.StatusNotFound})
	name := "test"
	image, err := client.InspectImage(name)
	if image != nil {
		t.Errorf("InspectImage(%q): expected <nil> image, got %#v.", name, image)
	}
	if !errors.Is(err, ErrNoSuchImage) {
		t.Errorf("InspectImage(%q): wrong error. Want %#v. Got %#v.", name, ErrNoSuchImage, err)
	}
}

func TestPushImage(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "Pushing 1/100", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	err := client.PushImage(PushImageOptions{Name: "test", OutputStream: &buf}, AuthConfiguration{})
	if err != nil {
		t.Fatal(err)
	}
	expected := "Pushing 1/100"
	if buf.String() != expected {
		t.Errorf("PushImage: Wrong output. Want %q. Got %q.", expected, buf.String())
	}
	req := fakeRT.requests[0]
	if req.Method != http.MethodPost {
		t.Errorf("PushImage: Wrong HTTP method. Want POST. Got %s.", req.Method)
	}
	u, _ := url.Parse(client.getURL("/images/test/push"))
	if req.URL.Path != u.Path {
		t.Errorf("PushImage: Wrong request path. Want %q. Got %q.", u.Path, req.URL.Path)
	}
	if query := req.URL.Query().Encode(); query != "" {
		t.Errorf("PushImage: Wrong query string. Want no parameters, got %q.", query)
	}

	authHeader, ok := req.Header["X-Registry-Auth"]
	if ok {
		t.Errorf("PushImage: unexpected non-empty X-Registry-Auth header: %v", authHeader)
	}
}

func TestPushImageWithRawJSON(t *testing.T) {
	t.Parallel()
	body := `
	{"status":"Pushing..."}
	{"status":"Pushing", "progress":"1/? (n/a)", "progressDetail":{"current":1}}}
	{"status":"Image successfully pushed"}
	`
	fakeRT := &FakeRoundTripper{
		message: body,
		status:  http.StatusOK,
		header: map[string]string{
			"Content-Type": "application/json",
		},
	}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer

	err := client.PushImage(PushImageOptions{
		Name:          "test",
		OutputStream:  &buf,
		RawJSONStream: true,
	}, AuthConfiguration{})
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != body {
		t.Errorf("PushImage: Wrong raw output. Want %q. Got %q.", body, buf.String())
	}
}

func TestPushImageWithAuthentication(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "Pushing 1/100", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	inputAuth := AuthConfiguration{
		Username: "gopher",
		Password: "gopher123",
		Email:    "gopher@tsuru.io",
	}
	err := client.PushImage(PushImageOptions{Name: "test", OutputStream: &buf}, inputAuth)
	if err != nil {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	var gotAuth AuthConfiguration

	auth, err := base64.URLEncoding.DecodeString(req.Header.Get("X-Registry-Auth"))
	if err != nil {
		t.Errorf("PushImage: caught error decoding auth. %#v", err.Error())
	}

	err = json.Unmarshal(auth, &gotAuth)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotAuth, inputAuth) {
		t.Errorf("PushImage: wrong auth configuration. Want %#v. Got %#v.", inputAuth, gotAuth)
	}
}

func TestPushImageCustomRegistry(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "Pushing 1/100", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var authConfig AuthConfiguration
	var buf bytes.Buffer
	opts := PushImageOptions{
		Name: "test", Registry: "docker.tsuru.io",
		OutputStream: &buf,
	}
	err := client.PushImage(opts, authConfig)
	if err != nil {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	expectedQuery := "registry=docker.tsuru.io"
	if query := req.URL.Query().Encode(); query != expectedQuery {
		t.Errorf("PushImage: Wrong query string. Want %q. Got %q.", expectedQuery, query)
	}
}

func TestPushImageNoName(t *testing.T) {
	t.Parallel()
	client := Client{}
	err := client.PushImage(PushImageOptions{}, AuthConfiguration{})
	if !errors.Is(err, ErrNoSuchImage) {
		t.Errorf("PushImage: got wrong error. Want %#v. Got %#v.", ErrNoSuchImage, err)
	}
}

func TestPullImage(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "Pulling 1/100", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	err := client.PullImage(PullImageOptions{Repository: "base", OutputStream: &buf},
		AuthConfiguration{})
	if err != nil {
		t.Fatal(err)
	}
	expected := "Pulling 1/100"
	if buf.String() != expected {
		t.Errorf("PullImage: Wrong output. Want %q. Got %q.", expected, buf.String())
	}
	req := fakeRT.requests[0]
	if req.Method != http.MethodPost {
		t.Errorf("PullImage: Wrong HTTP method. Want POST. Got %s.", req.Method)
	}
	u, _ := url.Parse(client.getURL("/images/create"))
	if req.URL.Path != u.Path {
		t.Errorf("PullImage: Wrong request path. Want %q. Got %q.", u.Path, req.URL.Path)
	}
	expectedQuery := "fromImage=base"
	if query := req.URL.Query().Encode(); query != expectedQuery {
		t.Errorf("PullImage: Wrong query strin. Want %q. Got %q.", expectedQuery, query)
	}
}

func TestPullImageWithDigest(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "Pulling 1/100", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	err := client.PullImage(PullImageOptions{
		Repository:   "tsuru/bs:latest@sha256:504a2f04aa5d07768e4f7467ddd2618b07dd6013cfabca7dc527a3d9fa786580",
		OutputStream: &buf,
	}, AuthConfiguration{})
	if err != nil {
		t.Fatal(err)
	}
	expected := "Pulling 1/100"
	if buf.String() != expected {
		t.Errorf("PullImage: Wrong output. Want %q. Got %q.", expected, buf.String())
	}
	req := fakeRT.requests[0]
	if req.Method != http.MethodPost {
		t.Errorf("PullImage: Wrong HTTP method. Want POST. Got %s.", req.Method)
	}
	u, _ := url.Parse(client.getURL("/images/create"))
	if req.URL.Path != u.Path {
		t.Errorf("PullImage: Wrong request path. Want %q. Got %q.", u.Path, req.URL.Path)
	}
	expectedQuery := url.Values{
		"fromImage": {"tsuru/bs:latest"},
		"tag":       {"sha256:504a2f04aa5d07768e4f7467ddd2618b07dd6013cfabca7dc527a3d9fa786580"},
	}
	if !reflect.DeepEqual(req.URL.Query(), expectedQuery) {
		t.Errorf("PullImage: Wrong query string\nWant %#v\nGot  %#v", expectedQuery, req.URL.Query())
	}
}

func TestPullImageWithDigestAndTag(t *testing.T) {
	// This is probably a wrong use of the Docker API, but let's let users
	// send the request to the API. And also changing this behavior would
	// be a breaking change on go-dockerclient.
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "Pulling 1/100", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	err := client.PullImage(PullImageOptions{
		Repository:   "tsuru/bs:latest@sha256:504a2f04aa5d07768e4f7467ddd2618b07dd6013cfabca7dc527a3d9fa786580",
		Tag:          "latest",
		OutputStream: &buf,
	}, AuthConfiguration{})
	if err != nil {
		t.Fatal(err)
	}
	expected := "Pulling 1/100"
	if buf.String() != expected {
		t.Errorf("PullImage: Wrong output. Want %q. Got %q.", expected, buf.String())
	}
	req := fakeRT.requests[0]
	if req.Method != http.MethodPost {
		t.Errorf("PullImage: Wrong HTTP method. Want POST. Got %s.", req.Method)
	}
	u, _ := url.Parse(client.getURL("/images/create"))
	if req.URL.Path != u.Path {
		t.Errorf("PullImage: Wrong request path. Want %q. Got %q.", u.Path, req.URL.Path)
	}
	expectedQuery := url.Values{
		"fromImage": {"tsuru/bs:latest@sha256:504a2f04aa5d07768e4f7467ddd2618b07dd6013cfabca7dc527a3d9fa786580"},
		"tag":       {"latest"},
	}
	if !reflect.DeepEqual(req.URL.Query(), expectedQuery) {
		t.Errorf("PullImage: Wrong query string\nWant %#vGot  %#v", expectedQuery, req.URL.Query())
	}
}

func TestPullImageWithRawJSON(t *testing.T) {
	t.Parallel()
	body := `
	{"status":"Pulling..."}
	{"status":"Pulling", "progress":"1 B/ 100 B", "progressDetail":{"current":1, "total":100}}
	`
	fakeRT := &FakeRoundTripper{
		message: body,
		status:  http.StatusOK,
		header: map[string]string{
			"Content-Type": "application/json",
		},
	}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	err := client.PullImage(PullImageOptions{
		Repository:    "base",
		OutputStream:  &buf,
		RawJSONStream: true,
	}, AuthConfiguration{})
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != body {
		t.Errorf("PullImage: Wrong raw output. Want %q. Got %q", body, buf.String())
	}
}

func TestPullImageWithoutOutputStream(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "Pulling 1/100", status: http.StatusOK}
	client := newTestClient(fakeRT)
	opts := PullImageOptions{
		Repository: "base",
		Registry:   "docker.tsuru.io",
	}
	err := client.PullImage(opts, AuthConfiguration{})
	if err != nil {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	expected := map[string][]string{"fromImage": {"base"}, "registry": {"docker.tsuru.io"}}
	got := map[string][]string(req.URL.Query())
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("PullImage: wrong query string. Want %#v. Got %#v.", expected, got)
	}
}

func TestPullImageCustomRegistry(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "Pulling 1/100", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	opts := PullImageOptions{
		Repository:   "base",
		Registry:     "docker.tsuru.io",
		OutputStream: &buf,
	}
	err := client.PullImage(opts, AuthConfiguration{})
	if err != nil {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	expected := map[string][]string{"fromImage": {"base"}, "registry": {"docker.tsuru.io"}}
	got := map[string][]string(req.URL.Query())
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("PullImage: wrong query string. Want %#v. Got %#v.", expected, got)
	}
}

func TestPullImageTag(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "Pulling 1/100", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	opts := PullImageOptions{
		Repository:   "base",
		Registry:     "docker.tsuru.io",
		Tag:          "latest",
		OutputStream: &buf,
	}
	err := client.PullImage(opts, AuthConfiguration{})
	if err != nil {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	expected := map[string][]string{"fromImage": {"base"}, "registry": {"docker.tsuru.io"}, "tag": {"latest"}}
	got := map[string][]string(req.URL.Query())
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("PullImage: wrong query string. Want %#v. Got %#v.", expected, got)
	}
}

func TestPullImageNoRepository(t *testing.T) {
	t.Parallel()
	var opts PullImageOptions
	client := Client{}
	err := client.PullImage(opts, AuthConfiguration{})
	if !errors.Is(err, ErrNoSuchImage) {
		t.Errorf("PullImage: got wrong error. Want %#v. Got %#v.", ErrNoSuchImage, err)
	}
}

func TestImportImageFromUrl(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	opts := ImportImageOptions{
		Source:       "http://mycompany.com/file.tar",
		Repository:   "testimage",
		Tag:          "tag",
		OutputStream: &buf,
	}
	err := client.ImportImage(opts)
	if err != nil {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	expected := map[string][]string{"fromSrc": {opts.Source}, "repo": {opts.Repository}, "tag": {opts.Tag}}
	got := map[string][]string(req.URL.Query())
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("ImportImage: wrong query string. Want %#v. Got %#v.", expected, got)
	}
}

func TestImportImageFromInput(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	in := bytes.NewBufferString("tar content")
	var buf bytes.Buffer
	opts := ImportImageOptions{
		Source: "-", Repository: "testimage",
		InputStream: in, OutputStream: &buf,
		Tag: "tag",
	}
	err := client.ImportImage(opts)
	if err != nil {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	expected := map[string][]string{"fromSrc": {opts.Source}, "repo": {opts.Repository}, "tag": {opts.Tag}}
	got := map[string][]string(req.URL.Query())
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("ImportImage: wrong query string. Want %#v. Got %#v.", expected, got)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Errorf("ImportImage: caugth error while reading body %#v", err.Error())
	}
	e := "tar content"
	if string(body) != e {
		t.Errorf("ImportImage: wrong body. Want %#v. Got %#v.", e, string(body))
	}
}

func TestImportImageDoesNotPassInputIfSourceIsNotDash(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	in := bytes.NewBufferString("foo")
	opts := ImportImageOptions{
		Source: "http://test.com/container.tar", Repository: "testimage",
		InputStream: in, OutputStream: &buf,
	}
	err := client.ImportImage(opts)
	if err != nil {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	expected := map[string][]string{"fromSrc": {opts.Source}, "repo": {opts.Repository}}
	got := map[string][]string(req.URL.Query())
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("ImportImage: wrong query string. Want %#v. Got %#v.", expected, got)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Errorf("ImportImage: caugth error while reading body %#v", err.Error())
	}
	if string(body) != "" {
		t.Errorf("ImportImage: wrong body. Want nothing. Got %#v.", string(body))
	}
}

func TestImportImageShouldPassTarContentToBodyWhenSourceIsFilePath(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	tarPath := "testing/data/container.tar"
	opts := ImportImageOptions{
		Source: tarPath, Repository: "testimage",
		OutputStream: &buf,
	}
	err := client.ImportImage(opts)
	if err != nil {
		t.Fatal(err)
	}
	tar, err := os.Open(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	tarContent, err := io.ReadAll(tar)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tarContent, body) {
		t.Errorf("ImportImage: wrong body. Want %#v content. Got %#v.", tarPath, body)
	}
}

func TestImportImageShouldChangeSourceToDashWhenItsAFilePath(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	tarPath := "testing/data/container.tar"
	opts := ImportImageOptions{
		Source: tarPath, Repository: "testimage",
		OutputStream: &buf,
	}
	err := client.ImportImage(opts)
	if err != nil {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	expected := map[string][]string{"fromSrc": {"-"}, "repo": {opts.Repository}}
	got := map[string][]string(req.URL.Query())
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("ImportImage: wrong query string. Want %#v. Got %#v.", expected, got)
	}
}

func TestBuildImageParameters(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	opts := BuildImageOptions{
		Name:                "testImage",
		NoCache:             true,
		CacheFrom:           []string{"test1", "test2"},
		SuppressOutput:      true,
		Pull:                true,
		RmTmpContainer:      true,
		ForceRmTmpContainer: true,
		Memory:              1024,
		Memswap:             2048,
		CPUShares:           10,
		CPUQuota:            7500,
		CPUPeriod:           100000,
		CPUSetCPUs:          "0-3",
		Ulimits:             []ULimit{{Name: "nofile", Soft: 100, Hard: 200}},
		BuildArgs:           []BuildArg{{Name: "SOME_VAR", Value: "some_value"}},
		InputStream:         &buf,
		OutputStream:        &buf,
		Labels:              map[string]string{"k": "v"},
		NetworkMode:         "host",
		CgroupParent:        "cgparent",
		SecurityOpt:         []string{"securityoptions"},
	}
	err := client.BuildImage(opts)
	if err != nil && !strings.Contains(err.Error(), "build image fail") {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	expected := map[string][]string{
		"t":            {opts.Name},
		"nocache":      {"1"},
		"cachefrom":    {`["test1","test2"]`},
		"q":            {"1"},
		"pull":         {"1"},
		"rm":           {"1"},
		"forcerm":      {"1"},
		"memory":       {"1024"},
		"memswap":      {"2048"},
		"cpushares":    {"10"},
		"cpuquota":     {"7500"},
		"cpuperiod":    {"100000"},
		"cpusetcpus":   {"0-3"},
		"labels":       {`{"k":"v"}`},
		"ulimits":      {`[{"Name":"nofile","Soft":100,"Hard":200}]`},
		"buildargs":    {`{"SOME_VAR":"some_value"}`},
		"networkmode":  {"host"},
		"cgroupparent": {"cgparent"},
		"securityopt":  {"securityoptions"},
	}
	got := map[string][]string(req.URL.Query())
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("BuildImage: wrong query string. Want %#v.\n Got %#v.", expected, got)
	}
	expectedPrefix := "http://localhost:4243/v1.25/"
	if !strings.HasPrefix(req.URL.String(), expectedPrefix) {
		t.Errorf("BuildImage: wrong URL version Want Prefix %s.\n Got URL: %s", expectedPrefix, req.URL.String())
	}
}

func TestBuildImageParametersForRemoteBuild(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	opts := BuildImageOptions{
		Name:           "testImage",
		Remote:         "testing/data/container.tar",
		SuppressOutput: true,
		OutputStream:   &buf,
	}
	err := client.BuildImage(opts)
	if err != nil {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	expected := map[string][]string{"t": {opts.Name}, "remote": {opts.Remote}, "q": {"1"}}
	got := map[string][]string(req.URL.Query())
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("BuildImage: wrong query string. Want %#v. Got %#v.", expected, got)
	}
}

func TestBuildImageMissingRepoAndNilInput(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	opts := BuildImageOptions{
		Name:           "testImage",
		SuppressOutput: true,
		OutputStream:   &buf,
	}
	err := client.BuildImage(opts)
	if !errors.Is(err, ErrMissingRepo) {
		t.Errorf("BuildImage: wrong error returned. Want %#v. Got %#v.", ErrMissingRepo, err)
	}
}

func TestBuildImageMissingOutputStream(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	opts := BuildImageOptions{Name: "testImage"}
	err := client.BuildImage(opts)
	if !errors.Is(err, ErrMissingOutputStream) {
		t.Errorf("BuildImage: wrong error returned. Want %#v. Got %#v.", ErrMissingOutputStream, err)
	}
}

func TestBuildImageWithRawJSON(t *testing.T) {
	t.Parallel()
	body := `
	{"stream":"Step 0 : FROM ubuntu:latest\n"}
	{"stream":" ---\u003e 4300eb9d3c8d\n"}
	{"stream":"Step 1 : MAINTAINER docker <eng@docker.com>\n"}
	{"stream":" ---\u003e Using cache\n"}
	{"stream":" ---\u003e 3a3ed758c370\n"}
	{"stream":"Step 2 : CMD /usr/bin/top\n"}
	{"stream":" ---\u003e Running in 36b1479cc2e4\n"}
	{"stream":" ---\u003e 4b6188aebe39\n"}
	{"stream":"Removing intermediate container 36b1479cc2e4\n"}
	{"stream":"Successfully built 4b6188aebe39\n"}
    `
	fakeRT := &FakeRoundTripper{
		message: body,
		status:  http.StatusOK,
		header: map[string]string{
			"Content-Type": "application/json",
		},
	}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	opts := BuildImageOptions{
		Name:           "testImage",
		RmTmpContainer: true,
		InputStream:    &buf,
		OutputStream:   &buf,
		RawJSONStream:  true,
	}
	err := client.BuildImage(opts)
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != body {
		t.Errorf("BuildImage: Wrong raw output. Want %q. Got %q.", body, buf.String())
	}
}

func TestBuildImageRemoteWithoutName(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	var buf bytes.Buffer
	opts := BuildImageOptions{
		Remote:         "testing/data/container.tar",
		SuppressOutput: true,
		OutputStream:   &buf,
	}
	err := client.BuildImage(opts)
	if err != nil {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	expected := map[string][]string{"t": {opts.Remote}, "remote": {opts.Remote}, "q": {"1"}}
	got := map[string][]string(req.URL.Query())
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("BuildImage: wrong query string. Want %#v. Got %#v.", expected, got)
	}
}

func TestTagImageParameters(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	opts := TagImageOptions{Repo: "testImage"}
	err := client.TagImage("base", opts)
	if err != nil && !strings.Contains(err.Error(), "tag image fail") {
		t.Fatal(err)
	}
	req := fakeRT.requests[0]
	expected := "http://localhost:4243/images/base/tag?repo=testImage"
	got := req.URL.String()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("TagImage: wrong query string. Want %#v. Got %#v.", expected, got)
	}
}

func TestTagImageMissingRepo(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	opts := TagImageOptions{Repo: "testImage"}
	err := client.TagImage("", opts)
	if !errors.Is(err, ErrNoSuchImage) {
		t.Errorf("TestTag: wrong error returned. Want %#v. Got %#v.",
			ErrNoSuchImage, err)
	}
}

func TestIsUrl(t *testing.T) {
	t.Parallel()
	url := "http://foo.bar/"
	result := isURL(url)
	if !result {
		t.Errorf("isURL: wrong match. Expected %#v to be a url. Got %#v.", url, result)
	}
	url = "/foo/bar.tar"
	result = isURL(url)
	if result {
		t.Errorf("isURL: wrong match. Expected %#v to not be a url. Got %#v", url, result)
	}
}

func TestLoadImage(t *testing.T) {
	t.Parallel()
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	tar, err := os.Open("testing/data/container.tar")
	if err != nil {
		t.Fatal(err)
	} else {
		defer tar.Close()
	}
	opts := LoadImageOptions{InputStream: tar}
	err = client.LoadImage(opts)
	if nil != err {
		t.Error(err)
	}
	req := fakeRT.requests[0]
	if req.Method != http.MethodPost {
		t.Errorf("LoadImage: wrong method. Expected %q. Got %q.", http.MethodPost, req.Method)
	}
	if req.URL.Path != "/images/load" {
		t.Errorf("LoadImage: wrong URL. Expected %q. Got %q.", "/images/load", req.URL.Path)
	}
}

func TestExportImage(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	opts := ExportImageOptions{Name: "testimage", OutputStream: &buf}
	err := client.ExportImage(opts)
	if nil != err {
		t.Error(err)
	}
	req := fakeRT.requests[0]
	if req.Method != http.MethodGet {
		t.Errorf("ExportImage: wrong method. Expected %q. Got %q.", http.MethodGet, req.Method)
	}
	expectedPath := "/images/testimage/get"
	if req.URL.Path != expectedPath {
		t.Errorf("ExportIMage: wrong path. Expected %q. Got %q.", expectedPath, req.URL.Path)
	}
}

func TestExportImages(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	client.requestedAPIVersion = apiVersion125
	opts := ExportImagesOptions{Names: []string{"testimage1", "testimage2:latest"}, OutputStream: &buf}
	err := client.ExportImages(opts)
	if nil != err {
		t.Error(err)
	}
	req := fakeRT.requests[0]
	if req.Method != http.MethodGet {
		t.Errorf("ExportImages: wrong method. Expected %q. Got %q.", http.MethodGet, req.Method)
	}
	expected := "http://localhost:4243/v1.25/images/get?names=testimage1%2Ctestimage2%3Alatest"
	got := req.URL.String()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("ExportImages: wrong path. Expected %q. Got %q.", expected, got)
	}

	client.requestedAPIVersion = apiVersion124
	err = client.ExportImages(opts)
	if nil != err {
		t.Error(err)
	}
	req = fakeRT.requests[1]
	expected = "http://localhost:4243/v1.24/images/get?names=testimage1&names=testimage2%3Alatest"
	got = req.URL.String()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("ExportImages: wrong path. Expected %q. Got %q.", expected, got)
	}
}

func TestExportImagesNoNames(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	fakeRT := &FakeRoundTripper{message: "", status: http.StatusOK}
	client := newTestClient(fakeRT)
	opts := ExportImagesOptions{Names: []string{}, OutputStream: &buf}
	err := client.ExportImages(opts)
	if err == nil {
		t.Error("Expected an error")
	}
	if !errors.Is(err, ErrMustSpecifyNames) {
		t.Error(err)
	}
}

func TestSearchImages(t *testing.T) {
	t.Parallel()
	body := `[
	{
		"description":"A container with Cassandra 2.0.3",
		"is_official":true,
		"is_automated":true,
		"name":"poklet/cassandra",
		"star_count":17
	},
	{
		"description":"A container with Cassandra 2.0.3",
		"is_official":true,
		"is_automated":false,
		"name":"poklet/cassandra",
		"star_count":17
	}
	,
	{
		"description":"A container with Cassandra 2.0.3",
		"is_official":false,
		"is_automated":true,
		"name":"poklet/cassandra",
		"star_count":17
	}
]`
	var expected []APIImageSearch
	err := json.Unmarshal([]byte(body), &expected)
	if err != nil {
		t.Fatal(err)
	}
	client := newTestClient(&FakeRoundTripper{message: body, status: http.StatusOK})
	result, err := client.SearchImages("cassandra")
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("SearchImages: Wrong return value. Want %#v. Got %#v.", expected, result)
	}
}

func TestSearchImagesEx(t *testing.T) {
	t.Parallel()
	body := `[
	{
		"description":"A container with Cassandra 2.0.3",
		"is_official":true,
		"is_automated":true,
		"name":"poklet/cassandra",
		"star_count":17
	},
	{
		"description":"A container with Cassandra 2.0.3",
		"is_official":true,
		"is_automated":false,
		"name":"poklet/cassandra",
		"star_count":17
	}
	,
	{
		"description":"A container with Cassandra 2.0.3",
		"is_official":false,
		"is_automated":true,
		"name":"poklet/cassandra",
		"star_count":17
	}
]`
	var expected []APIImageSearch
	err := json.Unmarshal([]byte(body), &expected)
	if err != nil {
		t.Fatal(err)
	}
	client := newTestClient(&FakeRoundTripper{message: body, status: http.StatusOK})
	auth := AuthConfiguration{}
	result, err := client.SearchImagesEx("cassandra", auth)
	if err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("SearchImages: Wrong return value. Want %#v. Got %#v.", expected, result)
	}
}

func TestPruneImages(t *testing.T) {
	t.Parallel()
	results := `{
		"ImagesDeleted": [
			{"Deleted": "a"},
			{"Deleted": "b"},
			{"Deleted": "c"}
		],
		"SpaceReclaimed": 123
	}`

	expected := &PruneImagesResults{}
	err := json.Unmarshal([]byte(results), expected)
	if err != nil {
		t.Fatal(err)
	}
	client := newTestClient(&FakeRoundTripper{message: results, status: http.StatusOK})
	got, err := client.PruneImages(PruneImagesOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("PruneImages: Expected %#v. Got %#v.", expected, got)
	}
}
