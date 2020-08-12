/*
Copyright (c) 2018 The Helm Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"image/color"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/disintegration/imaging"
	"github.com/kubeapps/common/datastore"
	"github.com/kubeapps/common/datastore/mockstore"
	"github.com/kubeapps/kubeapps/pkg/chart/models"
	"github.com/kubeapps/kubeapps/pkg/dbutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type bodyAPIListResponse struct {
	Data *apiListResponse `json:"data"`
	Meta meta             `json:"meta,omitempty"`
}

type bodyAPIResponse struct {
	Data apiResponse `json:"data"`
}

var chartsList []*models.Chart
var cc count

const (
	testChartReadme = "# Quickstart\n\n```bash\nhelm install my-repo/my-chart\n```"
	testChartValues = "image:\n  registry: docker.io\n  repository: my-repo/my-chart\n  tag: 0.1.0"
	testChartSchema = `{"properties": {"type": "object"}}`
	namespace       = "kubeapps-namespace"
	testRepoName    = "my-repo"
)

var testRepo *models.Repo = &models.Repo{Name: testRepoName, Namespace: namespace}

func iconBytes() []byte {
	var b bytes.Buffer
	img := imaging.New(1, 1, color.White)
	imaging.Encode(&b, img, imaging.PNG)
	return b.Bytes()
}

func getMockManager(m *mock.Mock) *mongodbAssetManager {
	dbSession := mockstore.NewMockSession(m)
	man := dbutils.NewMongoDBManager(datastore.Config{}, "kubeapps")
	man.DBSession = dbSession
	return &mongodbAssetManager{man}
}

func Test_chartAttributes(t *testing.T) {
	tests := []struct {
		name  string
		chart models.Chart
	}{
		{"chart has no icon", models.Chart{
			ID: "stable/wordpress",
		}},
		{"chart has a icon", models.Chart{
			ID: "repo/mychart", RawIcon: iconBytes(), IconContentType: "image/svg",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := chartAttributes(namespace, tt.chart)
			assert.Equal(t, tt.chart.ID, c.ID)
			assert.Equal(t, tt.chart.RawIcon, c.RawIcon)
			if len(tt.chart.RawIcon) == 0 {
				assert.Equal(t, len(c.Icon), 0, "icon url should be undefined")
			} else {
				assert.Equal(t, c.Icon, pathPrefix+"/ns/"+namespace+"/assets/"+tt.chart.ID+"/logo", "the icon url should be the same")
				assert.Equal(t, c.IconContentType, tt.chart.IconContentType, "the icon content type should be the same")
			}
		})
	}
}

func Test_chartVersionAttributes(t *testing.T) {
	tests := []struct {
		name       string
		chart      models.Chart
		chartFiles models.ChartFiles
		valuesName string
	}{
		{
			name:       "my-chart",
			chart:      models.Chart{ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.1.0"}}},
			chartFiles: models.ChartFiles{Values: "best values ever"},
			valuesName: "values.yaml",
		},
		{
			name:       "my-chart",
			chart:      models.Chart{ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.1.0"}}},
			chartFiles: models.ChartFiles{ValueFiles: []models.ValueFile{{Name: "values-test.yaml", Content: "best values ever"}}},
			valuesName: "values-test.yaml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m mock.Mock
			manager = getMockManager(&m)

			m.On("One", mock.Anything).Run(func(args mock.Arguments) {
				*args.Get(0).(*models.ChartFiles) = tt.chartFiles
			})

			cv := chartVersionAttributes(namespace, tt.chart.ID, tt.chart.ChartVersions[0])
			assert.Equal(t, cv.Version, tt.chart.ChartVersions[0].Version, "version string should be the same")
			assert.Equal(t, cv.Readme, pathPrefix+"/ns/"+namespace+"/assets/"+tt.chart.ID+"/versions/"+tt.chart.ChartVersions[0].Version+"/README.md", "README.md resource path should be the same")
			assert.Equal(t, cv.Values, pathPrefix+"/ns/"+namespace+"/assets/"+tt.chart.ID+"/versions/"+tt.chart.ChartVersions[0].Version+"/values/"+tt.valuesName, "values.yaml resource path should be the same")
		})
	}
}

func Test_newChartResponse(t *testing.T) {
	tests := []struct {
		name  string
		chart models.Chart
	}{
		{"chart has only one version", models.Chart{
			Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "1.2.3"}}},
		},
		{"chart has many versions", models.Chart{
			Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.1.2"}, {Version: "0.1.0"}},
		}},
		{"raw_icon is never sent down the wire", models.Chart{
			Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "1.2.3"}}, RawIcon: iconBytes(), IconContentType: "image/svg",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cResponse := newChartResponse(&tt.chart)
			assert.Equal(t, cResponse.Type, "chart", "response type is chart")
			assert.Equal(t, cResponse.ID, tt.chart.ID, "chart ID should be the same")
			assert.Equal(t, cResponse.Relationships["latestChartVersion"].Data.(models.ChartVersion).Version, tt.chart.ChartVersions[0].Version, "latestChartVersion should match version at index 0")
			assert.Equal(t, cResponse.Links.(selfLink).Self, pathPrefix+"/ns/"+namespace+"/charts/"+tt.chart.ID, "self link should be the same")
			// We don't send the raw icon down the wire.
			assert.Nil(t, cResponse.Attributes.(models.Chart).RawIcon)
		})
	}
}

func Test_newChartListResponse(t *testing.T) {
	tests := []struct {
		name   string
		input  []*models.Chart
		result []*models.Chart
	}{
		{"no charts", []*models.Chart{}, []*models.Chart{}},
		{"has one chart", []*models.Chart{
			{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.0.1", Digest: "123"}}},
		}, []*models.Chart{
			{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.0.1", Digest: "123"}}},
		}},
		{"has two charts", []*models.Chart{
			{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.0.1", Digest: "123"}}},
			{Repo: testRepo, ID: "stable/wordpress", ChartVersions: []models.ChartVersion{{Version: "1.2.3", Digest: "1234"}, {Version: "1.2.2", Digest: "12345"}}},
		}, []*models.Chart{
			{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.0.1", Digest: "123"}}},
			{Repo: testRepo, ID: "stable/wordpress", ChartVersions: []models.ChartVersion{{Version: "1.2.3", Digest: "1234"}, {Version: "1.2.2", Digest: "12345"}}},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clResponse := newChartListResponse(tt.input)
			assert.Equal(t, len(clResponse), len(tt.result), "number of charts in response should be the same")
			for i := range tt.result {
				assert.Equal(t, clResponse[i].Type, "chart", "response type is chart")
				assert.Equal(t, clResponse[i].ID, tt.result[i].ID, "chart ID should be the same")
				assert.Equal(t, clResponse[i].Relationships["latestChartVersion"].Data.(models.ChartVersion).Version, tt.result[i].ChartVersions[0].Version, "latestChartVersion should match version at index 0")
				assert.Equal(t, clResponse[i].Links.(selfLink).Self, pathPrefix+"/ns/"+namespace+"/charts/"+tt.result[i].ID, "self link should be the same")
			}
		})
	}
}

func Test_newChartVersionResponse(t *testing.T) {
	tests := []struct {
		name         string
		chart        models.Chart
		expectedIcon string
	}{
		{
			name: "my-chart",
			chart: models.Chart{
				Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.1.0"}, {Version: "0.2.3"}},
			},
		},
		{
			name: "RawIcon is never sent down the wire",
			chart: models.Chart{
				Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "1.2.3"}}, RawIcon: iconBytes(), IconContentType: "image/svg",
			},
			expectedIcon: "/v1/ns/" + namespace + "/assets/my-repo/my-chart/logo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range tt.chart.ChartVersions {
				cvResponse := newChartVersionResponse(&tt.chart, tt.chart.ChartVersions[i])
				assert.Equal(t, cvResponse.Type, "chartVersion", "response type is chartVersion")
				assert.Equal(t, cvResponse.ID, tt.chart.ID+"-"+tt.chart.ChartVersions[i].Version, "reponse id should have chart version suffix")
				assert.Equal(t, cvResponse.Links.(interface{}).(selfLink).Self, pathPrefix+"/ns/"+namespace+"/charts/"+tt.chart.ID+"/versions/"+tt.chart.ChartVersions[i].Version, "self link should be the same")
				assert.Equal(t, cvResponse.Attributes.(models.ChartVersion).Version, tt.chart.ChartVersions[i].Version, "chart version in the response should be the same")

				// The chart should have had its icon url set and raw icon data removed.
				expectedChart := tt.chart
				expectedChart.RawIcon = nil
				expectedChart.Icon = tt.expectedIcon
				expectedChart.ChartVersions = []models.ChartVersion{}
				assert.Equal(t, cvResponse.Relationships["chart"].Data.(interface{}).(models.Chart), expectedChart, "chart in relatioship matches")
			}
		})
	}
}

func Test_newChartVersionListResponse(t *testing.T) {
	tests := []struct {
		name  string
		chart models.Chart
	}{
		{"chart has no versions", models.Chart{
			Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{},
		}},
		{"chart has one version", models.Chart{
			Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.0.1"}},
		}},
		{"chart has many versions", models.Chart{
			Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.0.1"}, {Version: "0.0.2"}},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cvListResponse := newChartVersionListResponse(&tt.chart)
			assert.Equal(t, len(cvListResponse), len(tt.chart.ChartVersions), "number of chart versions in response should be the same")
			for i := range tt.chart.ChartVersions {
				assert.Equal(t, cvListResponse[i].Type, "chartVersion", "response type is chartVersion")
				assert.Equal(t, cvListResponse[i].ID, tt.chart.ID+"-"+tt.chart.ChartVersions[i].Version, "reponse id should have chart version suffix")
				assert.Equal(t, cvListResponse[i].Links.(interface{}).(selfLink).Self, pathPrefix+"/ns/"+namespace+"/charts/"+tt.chart.ID+"/versions/"+tt.chart.ChartVersions[i].Version, "self link should be the same")
				assert.Equal(t, cvListResponse[i].Attributes.(models.ChartVersion).Version, tt.chart.ChartVersions[i].Version, "chart version in the response should be the same")
			}
		})
	}
}

func Test_listCharts(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		charts     []*models.Chart
		chartFiles *models.ChartFiles
		meta       meta
	}{
		{
			name:       "no charts",
			query:      "",
			charts:     []*models.Chart{},
			chartFiles: nil,
			meta:       meta{1},
		},
		{
			name:       "one chart",
			charts:     []*models.Chart{{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.0.1", Digest: "123"}}}},
			chartFiles: &models.ChartFiles{Values: "best values ever"},
			meta:       meta{1},
		},
		{
			name: "two charts",
			charts: []*models.Chart{
				{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.0.1", Digest: "123"}}},
				{Repo: testRepo, ID: "stable/dokuwiki", ChartVersions: []models.ChartVersion{{Version: "1.2.3", Digest: "1234"}, {Version: "1.2.2", Digest: "12345"}}},
			},
			chartFiles: &models.ChartFiles{Values: "best values ever"},
			meta:       meta{1},
		},
		// Pagination tests
		{
			name:  "four charts with pagination",
			query: "?size=2",
			charts: []*models.Chart{
				{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.0.1", Digest: "123"}}},
				{Repo: testRepo, ID: "stable/dokuwiki", ChartVersions: []models.ChartVersion{{Version: "1.2.3", Digest: "1234"}}},
				{Repo: testRepo, ID: "stable/drupal", ChartVersions: []models.ChartVersion{{Version: "1.2.3", Digest: "12345"}}},
				{Repo: testRepo, ID: "stable/wordpress", ChartVersions: []models.ChartVersion{{Version: "1.2.3", Digest: "123456"}}},
			},
			chartFiles: &models.ChartFiles{Values: "best values ever"},
			meta:       meta{2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m mock.Mock
			manager = getMockManager(&m)

			if tt.chartFiles != nil {
				m.On("One", &models.ChartFiles{}).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.ChartFiles) = *tt.chartFiles
				})
			}

			m.On("All", &chartsList).Run(func(args mock.Arguments) {
				*args.Get(0).(*[]*models.Chart) = tt.charts
			})
			if tt.query != "" {
				m.On("One", &cc).Run(func(args mock.Arguments) {
					*args.Get(0).(*count) = count{len(tt.charts)}
				})
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/charts"+tt.query, nil)
			listCharts(w, req, Params{"namespace": namespace})

			m.AssertExpectations(t)
			assert.Equal(t, http.StatusOK, w.Code)

			var b bodyAPIListResponse
			json.NewDecoder(w.Body).Decode(&b)
			if b.Data == nil {
				t.Fatal("chart list shouldn't be null")
			}
			data := *b.Data
			assert.Len(t, data, len(tt.charts))
			for i, resp := range data {
				assert.Equal(t, resp.ID, tt.charts[i].ID, "chart id in the response should be the same")
				assert.Equal(t, resp.Type, "chart", "response type is chart")
				assert.Equal(t, resp.Links.(map[string]interface{})["self"], pathPrefix+"/ns/"+namespace+"/charts/"+tt.charts[i].ID, "self link should be the same")
				assert.Equal(t, resp.Relationships["latestChartVersion"].Data.(map[string]interface{})["version"], tt.charts[i].ChartVersions[0].Version, "latestChartVersion should match version at index 0")
			}
			assert.Equal(t, b.Meta, tt.meta, "response meta should be the same")
		})
	}
}

func Test_listRepoCharts(t *testing.T) {
	tests := []struct {
		name   		string
		repo   		string
		query  		string
		charts 		[]*models.Chart
		meta   		meta
		chartFiles 	*models.ChartFiles
	}{
		{
			name:   "repo has no charts",
			repo:   "my-repo",
			charts: []*models.Chart{},
			meta:   meta{1},
		},
		{
			name: "repo has one chart",
			repo: "my-repo",
			charts: []*models.Chart{
				{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.0.1", Digest: "123"}}},
			},
			meta: meta{1},
			chartFiles: &models.ChartFiles{Values: "best values ever"},
		},
		{
			name: "repo has many charts",
			repo: "my-repo",

			charts: []*models.Chart{
				{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.0.1", Digest: "123"}}},
				{Repo: testRepo, ID: "my-repo/dokuwiki", ChartVersions: []models.ChartVersion{{Version: "1.2.3", Digest: "1234"}, {Version: "1.2.2", Digest: "12345"}}},
			},
			meta: meta{1},
			chartFiles: &models.ChartFiles{Values: "best values ever"},
		},
		{
			name:  "repo has many charts with pagination",
			repo:  "my-repo",
			query: "?size=2",
			charts: []*models.Chart{
				{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.0.1", Digest: "123"}}},
				{Repo: testRepo, ID: "stable/dokuwiki", ChartVersions: []models.ChartVersion{{Version: "1.2.3", Digest: "1234"}}},
				{Repo: testRepo, ID: "stable/drupal", ChartVersions: []models.ChartVersion{{Version: "1.2.3", Digest: "12345"}}},
				{Repo: testRepo, ID: "stable/wordpress", ChartVersions: []models.ChartVersion{{Version: "1.2.3", Digest: "123456"}}},
			},
			meta: meta{2},
			chartFiles: &models.ChartFiles{Values: "best values ever"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m mock.Mock
			manager = getMockManager(&m)

			if tt.chartFiles != nil {
				m.On("One", &models.ChartFiles{}).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.ChartFiles) = *tt.chartFiles
				})
			}

			m.On("All", &chartsList).Run(func(args mock.Arguments) {
				*args.Get(0).(*[]*models.Chart) = tt.charts
			})
			if tt.query != "" {
				m.On("One", &cc).Run(func(args mock.Arguments) {
					*args.Get(0).(*count) = count{len(tt.charts)}
				})
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/charts/"+tt.repo+tt.query, nil)
			params := Params{
				"repo": "my-repo",
			}

			listCharts(w, req, params)

			m.AssertExpectations(t)
			assert.Equal(t, http.StatusOK, w.Code)

			var b bodyAPIListResponse
			json.NewDecoder(w.Body).Decode(&b)
			data := *b.Data
			assert.Len(t, data, len(tt.charts))
			for i, resp := range data {
				assert.Equal(t, resp.ID, tt.charts[i].ID, "chart id in the response should be the same")
				assert.Equal(t, resp.Type, "chart", "response type is chart")
				assert.Equal(t, resp.Relationships["latestChartVersion"].Data.(map[string]interface{})["version"], tt.charts[i].ChartVersions[0].Version, "latestChartVersion should match version at index 0")
			}
			assert.Equal(t, b.Meta, tt.meta, "response meta should be the same")
		})
	}
}

func Test_getChart(t *testing.T) {
	tests := []struct {
		name     	string
		err      	error
		chart    	models.Chart
		wantCode 	int
		chartFiles 	*models.ChartFiles
	}{
		{
			name:     "chart does not exist",
			err:      errors.New("return an error when checking if chart exists"),
			chart:    models.Chart{Repo: testRepo, ID: "my-repo/my-chart"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "chart exists",
			chart:    models.Chart{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.1.0"}}},
			wantCode: http.StatusOK,
			chartFiles: &models.ChartFiles{Values: "best values ever"},
		},
		{
			name:     "chart has multiple versions",
			chart:    models.Chart{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.1.0"}, {Version: "0.0.1"}}},
			wantCode: http.StatusOK,
			chartFiles: &models.ChartFiles{Values: "best values ever"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m mock.Mock
			manager = getMockManager(&m)

			if tt.chartFiles != nil {
				m.On("One", &models.ChartFiles{}).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.ChartFiles) = *tt.chartFiles
				})
			}

			if tt.err != nil {
				m.On("One", mock.Anything).Return(tt.err)
			} else {
				m.On("One", &models.Chart{}).Return(nil).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.Chart) = tt.chart
				})
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/charts/"+tt.chart.ID, nil)
			parts := strings.Split(tt.chart.ID, "/")
			params := Params{
				"namespace": namespace,
				"repo":      parts[0],
				"chartName": parts[1],
			}

			getChart(w, req, params)

			m.AssertExpectations(t)
			assert.Equal(t, tt.wantCode, w.Code)
			if tt.wantCode == http.StatusOK {
				var b bodyAPIResponse
				json.NewDecoder(w.Body).Decode(&b)
				assert.Equal(t, b.Data.ID, tt.chart.ID, "chart id in the response should be the same")
				assert.Equal(t, b.Data.Type, "chart", "response type is chart")
				assert.Equal(t, b.Data.Links.(map[string]interface{})["self"], pathPrefix+"/ns/"+namespace+"/charts/"+tt.chart.ID, "self link should be the same")
				assert.Equal(t, b.Data.Relationships["latestChartVersion"].Data.(map[string]interface{})["version"], tt.chart.ChartVersions[0].Version, "latestChartVersion should match version at index 0")
			}
		})
	}
}

func Test_listChartVersions(t *testing.T) {
	tests := []struct {
		name     	string
		err      	error
		chart    	models.Chart
		wantCode 	int
		chartFiles 	*models.ChartFiles
	}{
		{
			name:     "chart does not exist",
			err:      errors.New("return an error when checking if chart exists"),
			chart:    models.Chart{Repo: testRepo, ID: "my-repo/my-chart"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "chart exists",
			chart:    models.Chart{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.1.0"}}},
			wantCode: http.StatusOK,
			chartFiles: &models.ChartFiles{Values: "best values ever"},
		},
		{
			name:     "chart has multiple versions",
			chart:    models.Chart{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.1.0"}, {Version: "0.0.1"}}},
			wantCode: http.StatusOK,
			chartFiles: &models.ChartFiles{Values: "best values ever"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m mock.Mock
			manager = getMockManager(&m)

			if tt.chartFiles != nil {
				m.On("One", &models.ChartFiles{}).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.ChartFiles) = *tt.chartFiles
				})
			}

			if tt.err != nil {
				m.On("One", mock.Anything).Return(tt.err)
			} else {
				m.On("One", &models.Chart{}).Return(nil).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.Chart) = tt.chart
				})
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/charts/"+tt.chart.ID+"/versions", nil)
			parts := strings.Split(tt.chart.ID, "/")
			params := Params{
				"repo":      parts[0],
				"chartName": parts[1],
			}

			listChartVersions(w, req, params)

			m.AssertExpectations(t)
			assert.Equal(t, tt.wantCode, w.Code)
			if tt.wantCode == http.StatusOK {
				var b bodyAPIListResponse
				json.NewDecoder(w.Body).Decode(&b)
				data := *b.Data
				for i, resp := range data {
					assert.Equal(t, resp.ID, tt.chart.ID+"-"+tt.chart.ChartVersions[i].Version, "chart id in the response should be the same")
					assert.Equal(t, resp.Type, "chartVersion", "response type is chartVersion")
					assert.Equal(t, resp.Attributes.(map[string]interface{})["version"], tt.chart.ChartVersions[i].Version, "chart version should match")
				}
			}
		})
	}
}

func Test_getChartVersion(t *testing.T) {
	tests := []struct {
		name     	string
		err      	error
		chart    	models.Chart
		wantCode 	int
		chartFiles 	*models.ChartFiles
	}{
		{
			name:     "chart does not exist",
			err:      errors.New("return an error when checking if chart exists"),
			chart:    models.Chart{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.1.0"}}},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "chart exists",
			chart:    models.Chart{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.1.0"}}},
			wantCode: http.StatusOK,
			chartFiles: &models.ChartFiles{Values: "best values ever"},
		},
		{
			name:     "chart has multiple versions",
			chart:    models.Chart{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.1.0"}, {Version: "0.0.1"}}},
			wantCode: http.StatusOK,
			chartFiles: &models.ChartFiles{Values: "best values ever"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m mock.Mock
			manager = getMockManager(&m)

			if tt.chartFiles != nil {
				m.On("One", &models.ChartFiles{}).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.ChartFiles) = *tt.chartFiles
				})
			}

			if tt.err != nil {
				m.On("One", mock.Anything).Return(tt.err)
			} else {
				m.On("One", &models.Chart{}).Return(nil).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.Chart) = tt.chart
				})
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/charts/"+tt.chart.ID+"/versions/"+tt.chart.ChartVersions[0].Version, nil)
			parts := strings.Split(tt.chart.ID, "/")
			params := Params{
				"repo":      parts[0],
				"chartName": parts[1],
				"version":   tt.chart.ChartVersions[0].Version,
			}

			getChartVersion(w, req, params)

			m.AssertExpectations(t)
			assert.Equal(t, tt.wantCode, w.Code)
			if tt.wantCode == http.StatusOK {
				var b bodyAPIResponse
				json.NewDecoder(w.Body).Decode(&b)
				assert.Equal(t, b.Data.ID, tt.chart.ID+"-"+tt.chart.ChartVersions[0].Version, "chart id in the response should be the same")
				assert.Equal(t, b.Data.Type, "chartVersion", "response type is chartVersion")
				assert.Equal(t, b.Data.Attributes.(map[string]interface{})["version"], tt.chart.ChartVersions[0].Version, "chart version should match")
			}
		})
	}
}

func Test_getChartIcon(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		chart    models.Chart
		wantCode int
	}{
		{
			name:     "chart does not exist",
			err:      errors.New("return an error when checking if chart exists"),
			chart:    models.Chart{ID: "my-repo/my-chart"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "chart has icon",
			chart:    models.Chart{ID: "my-repo/my-chart", RawIcon: iconBytes(), IconContentType: "image/png"},
			wantCode: http.StatusOK,
		},
		{
			name:     "chart does not have a icon",
			chart:    models.Chart{ID: "my-repo/my-chart"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "chart has icon with custom type",
			chart:    models.Chart{ID: "my-repo/my-chart", RawIcon: iconBytes(), IconContentType: "image/svg"},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m mock.Mock
			manager = getMockManager(&m)

			if tt.err != nil {
				m.On("One", mock.Anything).Return(tt.err)
			} else {
				m.On("One", &models.Chart{}).Return(nil).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.Chart) = tt.chart
				})
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/assets/"+tt.chart.ID+"/logo", nil)
			parts := strings.Split(tt.chart.ID, "/")
			params := Params{
				"repo":      parts[0],
				"chartName": parts[1],
			}

			getChartIcon(w, req, params)

			m.AssertExpectations(t)
			assert.Equal(t, tt.wantCode, w.Code, "http status code should match")
			if tt.wantCode == http.StatusOK {
				assert.Equal(t, w.Body.Bytes(), tt.chart.RawIcon, "raw icon data should match")
				assert.Equal(t, w.Header().Get("Content-Type"), tt.chart.IconContentType, "icon content type should match")
			}
		})
	}
}

func Test_getChartVersionReadme(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		err      error
		files    models.ChartFiles
		wantCode int
	}{
		{
			name:     "chart does not exist",
			version:  "0.1.0",
			err:      errors.New("return an error when checking if chart exists"),
			files:    models.ChartFiles{ID: "my-repo/my-chart"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "chart exists",
			version:  "1.2.3",
			files:    models.ChartFiles{ID: "my-repo/my-chart", Readme: testChartReadme},
			wantCode: http.StatusOK,
		},
		{
			name:     "chart does not have a readme",
			version:  "1.1.1",
			files:    models.ChartFiles{ID: "my-repo/my-chart"},
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m mock.Mock
			manager = getMockManager(&m)

			if tt.err != nil {
				m.On("One", mock.Anything).Return(tt.err)
			} else {
				m.On("One", &models.ChartFiles{}).Return(nil).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.ChartFiles) = tt.files
				})
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/assets/"+tt.files.ID+"/versions/"+tt.version+"/README.md", nil)
			parts := strings.Split(tt.files.ID, "/")
			params := Params{
				"repo":      parts[0],
				"chartName": parts[1],
				"version":   "0.1.0",
			}

			getChartVersionReadme(w, req, params)

			m.AssertExpectations(t)
			assert.Equal(t, tt.wantCode, w.Code, "http status code should match")
			if tt.wantCode == http.StatusOK {
				assert.Equal(t, string(w.Body.Bytes()), tt.files.Readme, "content of the readme should match")
			}
		})
	}
}

func Test_getChartVersionValues(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		err      error
		files    models.ChartFiles
		wantCode int
	}{
		{
			name:     "chart does not exist",
			version:  "0.1.0",
			err:      errors.New("return an error when checking if chart exists"),
			files:    models.ChartFiles{ID: "my-repo/my-chart"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "chart exists",
			version:  "3.2.1",
			files:    models.ChartFiles{ID: "my-repo/my-chart", ValueFiles: []models.ValueFile{{Name: "values.yaml", Content: testChartValues}}},
			wantCode: http.StatusOK,
		},
		{
			name:     "chart does not have values.yaml",
			version:  "2.2.2",
			files:    models.ChartFiles{ID: "my-repo/my-chart"},
			wantCode: http.StatusOK,
		},
		{
			name:     "chart have a legacy values structure",
			version:  "3.2.1",
			files:    models.ChartFiles{ID: "my-repo/my-chart", Values: testChartValues},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m mock.Mock
			manager = getMockManager(&m)

			if tt.err != nil {
				m.On("One", mock.Anything).Return(tt.err)
			} else {
				m.On("One", &models.ChartFiles{}).Return(nil).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.ChartFiles) = tt.files
				})
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/assets/"+tt.files.ID+"/versions/"+tt.version+"/values/values.yaml", nil)
			parts := strings.Split(tt.files.ID, "/")
			params := Params{
				"repo":       parts[0],
				"chartName":  parts[1],
				"version":    "0.1.0",
				"valuesName": "values.yaml",
			}

			getChartVersionValues(w, req, params)

			m.AssertExpectations(t)
			assert.Equal(t, tt.wantCode, w.Code, "http status code should match")

			var comparableContent string

			if tt.err == nil {
				if len(tt.files.ValueFiles) > 0 {
					comparableContent = tt.files.ValueFiles[0].Content
				} else {
					comparableContent = tt.files.Values
				}
			}

			if tt.wantCode == http.StatusOK {
				assert.Equal(t, string(w.Body.Bytes()), comparableContent, "content of values.yaml should match")
			}
		})
	}
}

func Test_getChartVersionSchema(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		err      error
		files    models.ChartFiles
		wantCode int
	}{
		{
			name:     "chart does not exist",
			version:  "0.1.0",
			err:      errors.New("return an error when checking if chart exists"),
			files:    models.ChartFiles{ID: "my-repo/my-chart"},
			wantCode: http.StatusNotFound,
		},
		{
			name:     "chart exists",
			version:  "3.2.1",
			files:    models.ChartFiles{ID: "my-repo/my-chart", Schema: testChartSchema},
			wantCode: http.StatusOK,
		},
		{
			name:     "chart does not have values.yaml",
			version:  "2.2.2",
			files:    models.ChartFiles{ID: "my-repo/my-chart"},
			wantCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m mock.Mock
			manager = getMockManager(&m)

			if tt.err != nil {
				m.On("One", mock.Anything).Return(tt.err)
			} else {
				m.On("One", &models.ChartFiles{}).Return(nil).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.ChartFiles) = tt.files
				})
			}

			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/assets/"+tt.files.ID+"/versions/"+tt.version+"/values.schema.json", nil)
			parts := strings.Split(tt.files.ID, "/")
			params := Params{
				"repo":      parts[0],
				"chartName": parts[1],
				"version":   "0.1.0",
			}

			getChartVersionSchema(w, req, params)

			m.AssertExpectations(t)
			assert.Equal(t, tt.wantCode, w.Code, "http status code should match")
			if tt.wantCode == http.StatusOK {
				assert.Equal(t, string(w.Body.Bytes()), tt.files.Schema, "content of values.schema.json should match")
			}
		})
	}
}

func Test_findLatestChart(t *testing.T) {
	t.Run("returns mocked chart", func(t *testing.T) {
		chart := &models.Chart{
			Name: "foo",
			ID:   "foo",
			Repo: &models.Repo{Name: "bar"},
			ChartVersions: []models.ChartVersion{
				models.ChartVersion{Version: "1.0.0", AppVersion: "0.1.0"},
				models.ChartVersion{Version: "0.0.1", AppVersion: "0.1.0"},
			},
		}
		charts := []*models.Chart{chart}
		reqVersion := "1.0.0"
		reqAppVersion := "0.1.0"

		var m mock.Mock

		files := models.ChartFiles{Values: "best chart ever"}

		manager = getMockManager(&m)
		m.On("All", &chartsList).Run(func(args mock.Arguments) {
			*args.Get(0).(*[]*models.Chart) = charts
		})

		m.On("One", &models.ChartFiles{}).Run(func(args mock.Arguments) {
			*args.Get(0).(*models.ChartFiles) = files
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/charts?name="+chart.Name+"&version="+reqVersion+"&appversion="+reqAppVersion, nil)
		params := Params{
			"name":       chart.Name,
			"version":    reqVersion,
			"appversion": reqAppVersion,
		}

		listChartsWithFilters(w, req, params)

		var b bodyAPIListResponse
		json.NewDecoder(w.Body).Decode(&b)
		if b.Data == nil {
			t.Fatal("chart list shouldn't be null")
		}
		data := *b.Data

		if data[0].ID != chart.ID {
			t.Errorf("Expecting %v, received %v", chart, data[0].ID)
		}
	})
	t.Run("ignores duplicated chart", func(t *testing.T) {
		charts := []*models.Chart{
			{Name: "foo", ID: "stable/foo", Repo: &models.Repo{Name: "bar"}, ChartVersions: []models.ChartVersion{models.ChartVersion{Version: "1.0.0", AppVersion: "0.1.0", Digest: "123"}}},
			{Name: "foo", ID: "bitnami/foo", Repo: &models.Repo{Name: "bar"}, ChartVersions: []models.ChartVersion{models.ChartVersion{Version: "1.0.0", AppVersion: "0.1.0", Digest: "123"}}},
		}
		reqVersion := "1.0.0"
		reqAppVersion := "0.1.0"

		files := models.ChartFiles{Values: "best chart ever"}
		var m mock.Mock
		manager = getMockManager(&m)
		m.On("All", &chartsList).Run(func(args mock.Arguments) {
			*args.Get(0).(*[]*models.Chart) = charts
		})

		m.On("One", &models.ChartFiles{}).Run(func(args mock.Arguments) {
			*args.Get(0).(*models.ChartFiles) = files
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/charts?name="+charts[0].Name+"&version="+reqVersion+"&appversion="+reqAppVersion, nil)
		params := Params{
			"name":       charts[0].Name,
			"version":    reqVersion,
			"appversion": reqAppVersion,
		}

		listChartsWithFilters(w, req, params)

		var b bodyAPIListResponse
		json.NewDecoder(w.Body).Decode(&b)
		if b.Data == nil {
			t.Fatal("chart list shouldn't be null")
		}
		data := *b.Data

		assert.Equal(t, len(data), 1, "it should return a single chart")
		if data[0].ID != charts[0].ID {
			t.Errorf("Expecting %v, received %v", charts[0], data[0].ID)
		}
	})
	t.Run("includes duplicated charts when showDuplicates param set", func(t *testing.T) {
		charts := []*models.Chart{
			{Name: "foo", ID: "stable/foo", Repo: &models.Repo{Name: "bar"}, ChartVersions: []models.ChartVersion{models.ChartVersion{Version: "1.0.0", AppVersion: "0.1.0", Digest: "123"}}},
			{Name: "foo", ID: "bitnami/foo", Repo: &models.Repo{Name: "bar"}, ChartVersions: []models.ChartVersion{models.ChartVersion{Version: "1.0.0", AppVersion: "0.1.0", Digest: "123"}}},
		}
		reqVersion := "1.0.0"
		reqAppVersion := "0.1.0"

		files := models.ChartFiles{Values: "best chart ever"}
		var m mock.Mock
		manager = getMockManager(&m)
		m.On("All", &chartsList).Run(func(args mock.Arguments) {
			*args.Get(0).(*[]*models.Chart) = charts
		})

		m.On("One", &models.ChartFiles{}).Run(func(args mock.Arguments) {
			*args.Get(0).(*models.ChartFiles) = files
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/charts?showDuplicates=true&name="+charts[0].Name+"&version="+reqVersion+"&appversion="+reqAppVersion, nil)
		params := Params{
			"name":       charts[0].Name,
			"version":    reqVersion,
			"appversion": reqAppVersion,
		}

		listChartsWithFilters(w, req, params)

		var b bodyAPIListResponse
		json.NewDecoder(w.Body).Decode(&b)
		if b.Data == nil {
			t.Fatal("chart list shouldn't be null")
		}
		data := *b.Data

		assert.Equal(t, len(data), 2, "it should return both charts")
	})
}
