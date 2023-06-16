package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
)

func SendSms(phoneno, message string) {

	url := "https://api.netgsm.com.tr/sms/send/get"
	method := "POST"

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	_ = writer.WriteField("usercode", "8503051043")
	_ = writer.WriteField("password", "Oo_110308020")
	_ = writer.WriteField("gsmno", phoneno)
	_ = writer.WriteField("message", message)
	_ = writer.WriteField("msgheader", "8503051043")
	err := writer.Close()

	if err != nil {
		fmt.Println(err)
		return
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		fmt.Println(err)
		return
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer res.Body.Close()

	// read response body
	scanner := bufio.NewScanner(res.Body)
	var response []byte
	for scanner.Scan() {
		response = append(response, scanner.Bytes()...)
	}

	// print response body
	fmt.Println(string(response))

}
