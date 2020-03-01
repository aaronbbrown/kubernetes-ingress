// Copyright 2019 HAProxy Technologies LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controller

import (
	"fmt"
	"hash/fnv"
	"path"
	"strconv"
	"strings"

	"github.com/haproxytech/kubernetes-ingress/controller/utils"
	"github.com/haproxytech/models"
)

const (
	defaultCaptureLen = 128
)

func (c *HAProxyController) handleMaxconn(maxconn *int64, frontends ...string) error {
	for _, frontendName := range frontends {
		if frontend, err := c.frontendGet(frontendName); err == nil {
			frontend.Maxconn = maxconn
			err1 := c.frontendEdit(frontend)
			utils.LogErr(err1)
		} else {
			return err
		}
	}
	return nil
}

func (c *HAProxyController) handleRequestCapture(ingress *Ingress) error {
	//  Get and validate annotations
	annReqCapture, _ := GetValueFromAnnotations("request-capture", ingress.Annotations, c.cfg.ConfigMap.Annotations)
	annCaptureLen, _ := GetValueFromAnnotations("request-capture-len", ingress.Annotations, c.cfg.ConfigMap.Annotations)
	if annReqCapture == nil {
		return nil
	}
	var captureLen int64
	var err error
	if annCaptureLen != nil {
		captureLen, err = strconv.ParseInt(annCaptureLen.Value, 10, 64)
		if err != nil {
			captureLen = defaultCaptureLen
		}
		if annCaptureLen.Status == DELETED {
			captureLen = defaultCaptureLen
		}
	} else {
		captureLen = defaultCaptureLen
	}

	// Get Rules status
	status := ingress.Status
	if status == MODIFIED {
		if annReqCapture.Status != EMPTY {
			status = annReqCapture.Status
		}
	}

	// Update rules
	mapFiles := c.cfg.MapFiles
	for _, sample := range strings.Split(annReqCapture.Value, "\n") {
		key := hashStrToUint(fmt.Sprintf("RC-%s-%d", sample, captureLen))
		if status != EMPTY {
			mapFiles.Modified(key)
			c.cfg.HTTPRequestsStatus = MODIFIED
			c.cfg.TCPRequestsStatus = MODIFIED
			if status == DELETED {
				break
			}
		}
		if sample == "" {
			continue
		}
		for hostname := range ingress.Rules {
			mapFiles.AppendHost(key, hostname)
		}
		mapFile := path.Join(HAProxyMapDir, strconv.FormatUint(key, 10)) + ".lst"
		httpRule := models.HTTPRequestRule{
			ID:            utils.PtrInt64(0),
			Type:          "capture",
			CaptureSample: sample,
			Cond:          "if",
			CaptureLen:    captureLen,
			CondTest:      fmt.Sprintf("{ req.hdr(Host) -f %s }", mapFile),
		}
		tcpRule := models.TCPRequestRule{
			ID:       utils.PtrInt64(0),
			Type:     "content",
			Action:   "capture " + sample + " len " + strconv.FormatInt(captureLen, 10),
			Cond:     "if",
			CondTest: fmt.Sprintf("{ req_ssl_sni -f %s }", mapFile),
		}
		c.cfg.HTTPRequests[fmt.Sprint(key)] = []models.HTTPRequestRule{httpRule}
		c.cfg.TCPRequests[fmt.Sprint(key)] = []models.TCPRequestRule{tcpRule}
	}

	return err
}

func hashStrToUint(s string) uint64 {
	h := fnv.New64a()
	_, err := h.Write([]byte(strings.ToLower(s)))
	utils.LogErr(err)
	return h.Sum64()
}
