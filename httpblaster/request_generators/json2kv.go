package request_generators

import (
	"bufio"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/v3io/http_blaster/httpblaster/config"
	"github.com/v3io/http_blaster/httpblaster/igz_data"
	"io"
	"os"
	"runtime"
	"sync"
)

type Json2KV struct {
	workload config.Workload
	RequestCommon
}

func (self *Json2KV) UseCommon(c RequestCommon) {

}

func (self *Json2KV) generate_request(ch_records chan []byte, ch_req chan *Request, host string,
	wg *sync.WaitGroup) {
	defer wg.Done()
	parser := igz_data.EmdSchemaParser{}
	var contentType string = "text/html"
	e := parser.LoadSchema(self.workload.Schema)
	if e != nil {
		panic(e)
	}
	for r := range ch_records {
		json_payload, err := parser.EmdFromJsonRecord(r)
		if err != nil {
			panic(err)
		}
		req := AcquireRequest()
		self.PrepareRequest(contentType, self.workload.Header, "PUT",
			self.base_uri, json_payload, host, req.Request)
		ch_req <- req
	}
}

func (self *Json2KV) generate(ch_req chan *Request, payload string, host string) {
	defer close(ch_req)
	var ch_records chan []byte = make(chan []byte, 10000)

	wg := sync.WaitGroup{}
	wg.Add(runtime.NumCPU())
	for c := 0; c < runtime.NumCPU(); c++ {
		go self.generate_request(ch_records, ch_req, host, &wg)
	}
	ch_files := self.FilesScan(self.workload.Payload)

	for f := range ch_files {
		if file, err := os.Open(f); err == nil {
			reader := bufio.NewReader(file)
			var line_count int = 0
			for {
				line, err := reader.ReadBytes('\n')
				if err == nil {
					ch_records <- line
					line_count++
					if line_count%1024 == 0 {
						log.Printf("line: %d from file %s was submitted", line_count, f)
					}
				} else if err == io.EOF {
					break
				} else {
					log.Fatal(err)
				}
			}

			log.Println(fmt.Sprintf("Finish file scaning, generated %d records", line_count))
		} else {
			panic(err)
		}
	}
	close(ch_records)
	wg.Wait()
}

func (self *Json2KV) GenerateRequests(global config.Global, wl config.Workload, tls_mode bool, host string, ret_ch chan *Response, worker_qd int) chan *Request {
	self.workload = wl
	if self.workload.Header == nil {
		self.workload.Header = make(map[string]string)
	}
	self.workload.Header["X-v3io-function"] = "PutItem"

	ch_req := make(chan *Request, worker_qd)

	self.SetBaseUri(tls_mode, host, self.workload.Container, self.workload.Target)

	go self.generate(ch_req, self.workload.Payload, host)

	return ch_req
}
