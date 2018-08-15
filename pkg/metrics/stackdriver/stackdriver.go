// Copyright © 2018 Aviv Laufer <aviv.laufer@gmail.com>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package stackdriver

import (
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/doitintl/kuberbs/pkg/metrics"
	"github.com/doitintl/kuberbs/pkg/utils"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/monitoring/v3"
)

// Stackdriver - Holds data about the stackdriver watcher
type Stackdriver struct {
	StartAt        time.Time
	EndAt          time.Time
	MetricName     string
	metricsHandler metrics.ErrorRateHandler
	doneHandler    metrics.DoneHandler
	doneChan       chan bool
}

// NewStackDriver - create new stackdriver struct
func NewStackDriver(startat time.Time, endat time.Time, metricname string) *Stackdriver {
	return &Stackdriver{
		StartAt:    startat,
		EndAt:      endat,
		MetricName: metricname,
	}
}

// Start - start watching
func (sd *Stackdriver) Start() {
	logrus.Debug("Starting Stackdriver Run")
	delta := time.Until(sd.EndAt)
	sd.doneChan = make(chan bool)
	timeChan := time.NewTimer(delta).C
	tickChan := time.NewTicker(time.Second * metrics.CheckMetricsInterval).C
	for {
		select {
		case <-tickChan:
			logrus.Debug("Tick Called")
			rate, err := sd.checkMetrics()
			if err != nil {
				logrus.Fatal(err)
			} else {
				sd.metricsHandler(rate)
			}
		case <-timeChan:
			logrus.Debug("Timer expired. We are done!")
			sd.doneHandler(true)
			return
		case remove := <-sd.doneChan:
			logrus.Debug("Stop called. We are done!")
			sd.doneHandler(remove)
			return
		}
	}

}

// Stop - kill the watch time and stop watching
func (sd *Stackdriver) Stop(remove bool) {
	sd.doneChan <- remove
}

// checkMetrics - connect to stackdriver and call readTimeSeriesValue
func (sd *Stackdriver) checkMetrics() (float64, error) {
	ctx := context.Background()
	s, err := createService(ctx)
	if err != nil {
		logrus.Fatal(err)
		return 0, err
	}
	metricType := sd.MetricName
	return readTimeSeriesValue(s, metricType, sd.StartAt)

}

// SetMetricsHandler - set call back after reading the metrics
func (sd *Stackdriver) SetMetricsHandler(erh metrics.ErrorRateHandler) {
	sd.metricsHandler = erh
}

// SetDoneHandler - set the callback handler when watcher is done
func (sd *Stackdriver) SetDoneHandler(dh metrics.DoneHandler) {
	sd.doneHandler = dh

}

func readTimeSeriesValue(s *monitoring.Service, metricType string, startat time.Time) (float64, error) {
	projectID, err := utils.ProjectName()
	if err != nil {
		logrus.Error(err)
		//return 0, err
		//TODO REMOVE
		projectID = "aviv-playground"
	}
	logrus.Debugf("readTimeSeriesValue for %s", metricType)
	startTime := time.Now().UTC().Add(time.Until(startat))
	endTime := time.Now().UTC()
	resp, err := s.Projects.TimeSeries.List(utils.ProjectResource(projectID)).
		Filter(fmt.Sprintf("metric.type=\"%s\"", metricType)).
		IntervalStartTime(startTime.Format(time.RFC3339Nano)).
		IntervalEndTime(endTime.Format(time.RFC3339Nano)).
		Do()
	if err != nil {
		logrus.Error(err)
		return 0.0, fmt.Errorf("Could not read time series value, %v ", err)
	}
	errSum := int64(0)
	if len(resp.TimeSeries) != 0 {
		for _, p := range resp.TimeSeries[0].Points {
			errSum = errSum + *p.Value.Int64Value
		}
		logrus.Debugf("errSum %d", errSum)
		return float64(errSum) / endTime.Sub(startTime).Seconds(), nil
	}
	return 0, nil
}

func createService(ctx context.Context) (*monitoring.Service, error) {
	hc, err := google.DefaultClient(ctx, monitoring.MonitoringScope)
	if err != nil {
		return nil, err
	}
	s, err := monitoring.New(hc)
	if err != nil {
		logrus.Error(err)
		return nil, err
	}
	return s, nil
}
