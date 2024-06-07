package moodle

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
)

type TokenResponse struct {
	Token        string `json:"token"`
	PrivateToken string `json:"privatetoken"`
	Error        string `json:"error"`
	ErrorCode    string `json:"errorcode"`
}

type MoodleClient struct {
	ServiceUrl   string
	Token        string
	PrivateToken string
	Username     string
	SkipSSL      bool
	CourseApi    *CourseApi
	Jar          *cookiejar.Jar
	Client       *http.Client
}

func NewMoodleClient(skipSSL bool) *MoodleClient {
	if skipSSL {
		logrus.Info("Skipping SSL verification for all requests")
	}
	client := &MoodleClient{SkipSSL: skipSSL}
	client.CourseApi = newCourseApi(client)
	jar, err := cookiejar.New(nil)
	if err != nil {
		logrus.Fatal("Could not create Cookie jar", err)
	}
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipSSL},
	}

	client.Client = &http.Client{Transport: tr, Jar: jar}

	return client
}
func (mc *MoodleClient) Login(username string, password string) error {
	loginURL := fmt.Sprintf("%s/login/token.php", mc.ServiceUrl)
	data := url.Values{}
	data.Set("username", username)
	data.Set("password", password)
	data.Set("service", "moodle_mobile_app")
	req, err := http.NewRequest("POST", loginURL, nil)
	if err != nil {
		return err
	}
	req.URL.RawQuery = data.Encode()
	resp, err := mc.Client.Do(req)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {

		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return err
	}

	if tokenResp.Error != "" {
		return fmt.Errorf("failed to obtain token: %s", tokenResp.Error)
	}

	mc.Token = tokenResp.Token
	mc.PrivateToken = tokenResp.PrivateToken
	mc.Username = username
	if err != nil {
		logrus.Error("Failed to get Session. This is not a big problem but some endpoints will not work", err)
	}
	return nil
}

func (mc *MoodleClient) makeRequest(function string, params map[string]string, url string) ([]byte, error) {
	webserviceURL := fmt.Sprintf("%s%s", mc.ServiceUrl, url)

	params["wstoken"] = mc.Token
	params["wsfunction"] = function
	params["moodlewsrestformat"] = "json"
	req, err := http.NewRequest("GET", webserviceURL, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	for key, value := range params {
		q.Add(key, value)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := mc.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func (mc *MoodleClient) makeWebserviceRequest(function string, params map[string]string) ([]byte, error) {
	return mc.makeRequest(function, params, "/webservice/rest/server.php")
}

func (mc *MoodleClient) makeModRequest(function string, params map[string]string) ([]byte, error) {
	return mc.makeRequest(function, params, "/mod/assign/view.php")
}

func (mc *MoodleClient) DownloadFile(url string, path string, filesize int64) error {
	return mc.downloadFile(url, path, filesize)
}

func (mc *MoodleClient) downloadFile(url string, path string, filesize int64) error {

	fileInfo, err := os.Stat(path)
	if err == nil {
		if filesize == fileInfo.Size() {
			logrus.Info("Skip file download ", path)
			return nil
		}
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	q := req.URL.Query()
	q.Add("token", mc.Token)
	req.URL.RawQuery = q.Encode()

	resp, err := mc.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download file: status code %d", resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return err
	}
	outFile, err := os.Create(path)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
