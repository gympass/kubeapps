/*
Copyright (c) 2017 The Helm Authors

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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kubeapps/kubeapps/pkg/chart/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// tests the GET /live endpoint
func Test_GetLive(t *testing.T) {
	var m mock.Mock
	manager = getMockManager(&m)

	ts := httptest.NewServer(setupRoutes())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/live")
	assert.NoError(t, err, "should not return an error")
	defer res.Body.Close()
	assert.Equal(t, res.StatusCode, http.StatusOK, "http status code should match")
}

// tests the GET /ready endpoint
func Test_GetReady(t *testing.T) {
	var m mock.Mock
	manager = getMockManager(&m)

	ts := httptest.NewServer(setupRoutes())
	defer ts.Close()

	res, err := http.Get(ts.URL + "/ready")
	assert.NoError(t, err, "should not return an error")
	defer res.Body.Close()
	assert.Equal(t, res.StatusCode, http.StatusOK, "http status code should match")
}

// tests the GET /{apiVersion}/ns/{namespace}/charts endpoint
func Test_GetCharts(t *testing.T) {
	ts := httptest.NewServer(setupRoutes())
	defer ts.Close()

	tests := []struct {
		name       string
		charts     []*models.Chart
		chartFiles *models.ChartFiles
	}{
		{
			name:   "no charts",
			charts: []*models.Chart{},
		},
		{
			name: "one chart",
			charts: []*models.Chart{
				{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.0.1"}}},
			},
			chartFiles: &models.ChartFiles{Values: "best chart ever"},
		},
		{
			name: "two charts",
			charts: []*models.Chart{
				{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.0.1", Digest: "123"}}},
				{Repo: testRepo, ID: "my-repo/dokuwiki", ChartVersions: []models.ChartVersion{{Version: "1.2.3", Digest: "1234"}, {Version: "1.2.2", Digest: "12345"}}},
			},
			chartFiles: &models.ChartFiles{Values: "best chart ever"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m mock.Mock
			manager = getMockManager(&m)
			m.On("All", &chartsList).Run(func(args mock.Arguments) {
				*args.Get(0).(*[]*models.Chart) = tt.charts
			})

			if tt.chartFiles != nil {
				m.On("One", &models.ChartFiles{}).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.ChartFiles) = *tt.chartFiles
				})
			}

			res, err := http.Get(ts.URL + pathPrefix + "/ns/kubeapps/charts")
			assert.NoError(t, err)
			defer res.Body.Close()

			m.AssertExpectations(t)
			assert.Equal(t, res.StatusCode, http.StatusOK, "http status code should match")

			var b bodyAPIListResponse
			json.NewDecoder(res.Body).Decode(&b)
			assert.Len(t, *b.Data, len(tt.charts))
		})
	}
}

// tests the GET /{apiVersion}/ns/{namespace}/charts/{repo} endpoint
func Test_GetChartsInRepo(t *testing.T) {
	ts := httptest.NewServer(setupRoutes())
	defer ts.Close()

	tests := []struct {
		name       string
		repo       string
		charts     []*models.Chart
		chartFiles *models.ChartFiles
	}{
		{
			name:   "repo has no charts",
			repo:   "my-repo",
			charts: []*models.Chart{},
		},
		{
			name: "repo has one chart",
			repo: "my-repo",
			charts: []*models.Chart{
				{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.0.1", Digest: "123"}}},
			},
			chartFiles: &models.ChartFiles{Values: "best chart ever"},
		},
		{
			name: "repo has many charts",
			repo: "my-repo",
			charts: []*models.Chart{
				{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.0.1", Digest: "123"}}},
				{Repo: testRepo, ID: "my-repo/dokuwiki", ChartVersions: []models.ChartVersion{{Version: "1.2.3", Digest: "1234"}, {Version: "1.2.2", Digest: "12345"}}},
			},
			chartFiles: &models.ChartFiles{Values: "best chart ever"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m mock.Mock
			manager = getMockManager(&m)
			m.On("All", &chartsList).Run(func(args mock.Arguments) {
				*args.Get(0).(*[]*models.Chart) = tt.charts
			})

			if tt.chartFiles != nil {
				m.On("One", &models.ChartFiles{}).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.ChartFiles) = *tt.chartFiles
				})
			}

			res, err := http.Get(ts.URL + pathPrefix + "/ns/kubeapps/charts/" + tt.repo)
			assert.NoError(t, err)
			defer res.Body.Close()

			m.AssertExpectations(t)
			assert.Equal(t, res.StatusCode, http.StatusOK, "http status code should match")

			var b bodyAPIListResponse
			json.NewDecoder(res.Body).Decode(&b)
			assert.Len(t, *b.Data, len(tt.charts))
		})
	}
}

// tests the GET /{apiVersion}/ns/charts/{repo}/{chartName} endpoint
func Test_GetChartInRepo(t *testing.T) {
	ts := httptest.NewServer(setupRoutes())
	defer ts.Close()

	tests := []struct {
		name       string
		err        error
		chart      models.Chart
		wantCode   int
		chartFiles *models.ChartFiles
	}{
		{
			name:     "chart does not exist",
			err:      errors.New("return an error when checking if chart exists"),
			chart:    models.Chart{Repo: testRepo, ID: "my-repo/my-chart"},
			wantCode: http.StatusNotFound,
		},
		{
			name:       "chart exists",
			chart:      models.Chart{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.1.0"}}},
			wantCode:   http.StatusOK,
			chartFiles: &models.ChartFiles{Values: "best chart ever"},
		},
		{
			name:       "chart has multiple versions",
			chart:      models.Chart{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.1.0"}, {Version: "0.0.1"}}},
			wantCode:   http.StatusOK,
			chartFiles: &models.ChartFiles{Values: "best chart ever"},
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

			if tt.chartFiles != nil {
				m.On("One", &models.ChartFiles{}).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.ChartFiles) = *tt.chartFiles
				})
			}

			res, err := http.Get(ts.URL + pathPrefix + "/ns/kubeapps/charts/" + tt.chart.ID)
			assert.NoError(t, err)
			defer res.Body.Close()

			m.AssertExpectations(t)
			assert.Equal(t, res.StatusCode, tt.wantCode, "http status code should match")
		})
	}
}

// tests the GET /{apiVersion}/ns/charts/{repo}/{chartName}/versions endpoint
func Test_ListChartVersions(t *testing.T) {
	ts := httptest.NewServer(setupRoutes())
	defer ts.Close()

	tests := []struct {
		name       string
		err        error
		chart      models.Chart
		wantCode   int
		chartFiles *models.ChartFiles
	}{
		{
			name:     "chart does not exist",
			err:      errors.New("return an error when checking if chart exists"),
			chart:    models.Chart{Repo: testRepo, ID: "my-repo/my-chart"},
			wantCode: http.StatusNotFound,
		},
		{
			name:       "chart exists",
			chart:      models.Chart{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.1.0"}}},
			wantCode:   http.StatusOK,
			chartFiles: &models.ChartFiles{Values: "best chart ever"},
		},
		{
			name:       "chart has multiple versions",
			chart:      models.Chart{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.1.0"}, {Version: "0.0.1"}}},
			wantCode:   http.StatusOK,
			chartFiles: &models.ChartFiles{Values: "best chart ever"},
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

			if tt.chartFiles != nil {
				m.On("One", &models.ChartFiles{}).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.ChartFiles) = *tt.chartFiles
				})
			}

			res, err := http.Get(ts.URL + pathPrefix + "/ns/kubeapps/charts/" + tt.chart.ID + "/versions")
			assert.NoError(t, err)
			defer res.Body.Close()

			m.AssertExpectations(t)
			assert.Equal(t, res.StatusCode, tt.wantCode, "http status code should match")
		})
	}
}

// tests the GET /{apiVersion}/ns/charts/{repo}/{chartName}/versions/{:version} endpoint
func Test_GetChartVersion(t *testing.T) {
	ts := httptest.NewServer(setupRoutes())
	defer ts.Close()

	tests := []struct {
		name       string
		err        error
		chart      models.Chart
		wantCode   int
		chartFiles *models.ChartFiles
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
			chartFiles: &models.ChartFiles{Values: "best chart ever"},
		},
		{
			name:     "chart has multiple versions",
			chart:    models.Chart{Repo: testRepo, ID: "my-repo/my-chart", ChartVersions: []models.ChartVersion{{Version: "0.1.0"}, {Version: "0.0.1"}}},
			wantCode: http.StatusOK,
			chartFiles: &models.ChartFiles{Values: "best chart ever"},
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

			if tt.chartFiles != nil {
				m.On("One", &models.ChartFiles{}).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.ChartFiles) = *tt.chartFiles
				})
			}

			res, err := http.Get(ts.URL + pathPrefix + "/ns/kubeapps/charts/" + tt.chart.ID + "/versions/" + tt.chart.ChartVersions[0].Version)
			assert.NoError(t, err)
			defer res.Body.Close()

			m.AssertExpectations(t)
			assert.Equal(t, res.StatusCode, tt.wantCode, "http status code should match")
		})
	}
}

// tests the GET /{apiVersion}/ns/assets/{repo}/{chartName}/logo-160x160-fit.png endpoint
func Test_GetChartIcon(t *testing.T) {
	ts := httptest.NewServer(setupRoutes())
	defer ts.Close()

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
			chart:    models.Chart{ID: "my-repo/my-chart", RawIcon: iconBytes()},
			wantCode: http.StatusOK,
		},
		{
			name:     "chart does not have a icon",
			chart:    models.Chart{ID: "my-repo/my-chart"},
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
				m.On("One", &models.Chart{}).Return(nil).Run(func(args mock.Arguments) {
					*args.Get(0).(*models.Chart) = tt.chart
				})
			}

			res, err := http.Get(ts.URL + pathPrefix + "/ns/kubeapps/assets/" + tt.chart.ID + "/logo")
			assert.NoError(t, err)
			defer res.Body.Close()

			m.AssertExpectations(t)
			assert.Equal(t, res.StatusCode, tt.wantCode, "http status code should match")
		})
	}
}

// tests the GET /{apiVersion}/ns/assets/{repo}/{chartName}/versions/{version}/README.md endpoint
func Test_GetChartReadme(t *testing.T) {
	ts := httptest.NewServer(setupRoutes())
	defer ts.Close()

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

			res, err := http.Get(ts.URL + pathPrefix + "/ns/kubeapps/assets/" + tt.files.ID + "/versions/" + tt.version + "/README.md")
			assert.NoError(t, err)
			defer res.Body.Close()

			m.AssertExpectations(t)
			assert.Equal(t, tt.wantCode, res.StatusCode, "http status code should match")
		})
	}
}

// tests the GET /{apiVersion}/ns/assets/{repo}/{chartName}/versions/{version}/values.yaml endpoint
func Test_GetChartValues(t *testing.T) {
	ts := httptest.NewServer(setupRoutes())
	defer ts.Close()

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

			res, err := http.Get(ts.URL + pathPrefix + "/ns/kubeapps/assets/" + tt.files.ID + "/versions/" + tt.version + "/values/values.yaml")
			assert.NoError(t, err)
			defer res.Body.Close()

			m.AssertExpectations(t)
			assert.Equal(t, res.StatusCode, tt.wantCode, "http status code should match")
		})
	}
}

// tests the GET /{apiVersion}/ns/assets/{repo}/{chartName}/versions/{version}/values/schema.json endpoint
func Test_GetChartSchema(t *testing.T) {
	ts := httptest.NewServer(setupRoutes())
	defer ts.Close()

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
			name:     "chart does not have values.schema.json",
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

			res, err := http.Get(ts.URL + pathPrefix + "/ns/kubeapps/assets/" + tt.files.ID + "/versions/" + tt.version + "/values.schema.json")
			assert.NoError(t, err)
			defer res.Body.Close()

			m.AssertExpectations(t)
			assert.Equal(t, res.StatusCode, tt.wantCode, "http status code should match")
		})
	}
}
