//
// Copyright (C) 2022-2023 IOTech Ltd
//
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bootstrapContainer "github.com/edgexfoundry/go-mod-bootstrap/v2/bootstrap/container"
	bootstrapConfig "github.com/edgexfoundry/go-mod-bootstrap/v2/config"
	"github.com/edgexfoundry/go-mod-bootstrap/v2/di"
	"github.com/edgexfoundry/go-mod-core-contracts/v2/clients/logger"
	"github.com/edgexfoundry/go-mod-core-contracts/v2/common"
	commonDTO "github.com/edgexfoundry/go-mod-core-contracts/v2/dtos/common"
	"github.com/edgexfoundry/go-mod-core-contracts/v2/errors"
	v2Models "github.com/edgexfoundry/go-mod-core-contracts/v2/models"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/edgexfoundry/edgex-go/internal/core/keeper/config"
	"github.com/edgexfoundry/edgex-go/internal/core/keeper/constants"
	"github.com/edgexfoundry/edgex-go/internal/core/keeper/container"
	"github.com/edgexfoundry/edgex-go/internal/core/keeper/dtos"
	"github.com/edgexfoundry/edgex-go/internal/core/keeper/dtos/requests"
	"github.com/edgexfoundry/edgex-go/internal/core/keeper/dtos/responses"
	"github.com/edgexfoundry/edgex-go/internal/core/keeper/infrastructure/interfaces/mocks"
	"github.com/edgexfoundry/edgex-go/internal/core/keeper/models"
)

const testServiceId = "test-service"

func mockDic() *di.Container {
	return di.NewContainer(di.ServiceConstructorMap{
		container.ConfigurationName: func(get di.Get) interface{} {
			return &config.ConfigurationStruct{
				Writable: config.WritableInfo{
					LogLevel: "DEBUG",
				},
				Service: bootstrapConfig.ServiceInfo{
					MaxResultCount: 30,
				},
			}
		},
		bootstrapContainer.LoggingClientInterfaceName: func(get di.Get) interface{} {
			return logger.NewMockClient()
		},
	})
}

func buildTestRegistrationRequest() requests.AddRegistrationRequest {
	return requests.AddRegistrationRequest{
		BaseRequest: commonDTO.BaseRequest{
			Versionable: commonDTO.NewVersionable(),
			RequestId:   "",
		},
		Registration: dtos.Registration{
			ServiceId: testServiceId,
			Host:      "localhost",
			Port:      50000,
			HealthCheck: dtos.HealthCheck{
				Interval: "10s",
				Path:     "/api/v2/ping",
				Type:     "http",
			},
		},
	}
}

func TestRegistryController_Register(t *testing.T) {
	validReq := buildTestRegistrationRequest()
	validRegistrationModel := dtos.ToRegistrationModel(validReq.Registration)
	validRegistrationModel.Status = v2Models.Unknown
	duplicateServiceId := validReq
	duplicateServiceId.Registration.ServiceId = "duplicated"
	duplicateServiceId.Registration.Status = v2Models.Up
	emptyServiceId := validReq
	emptyServiceId.Registration.ServiceId = ""
	invalidInterval := validReq
	invalidInterval.Registration.HealthCheck.Interval = "10t"
	emptyHealthCheckType := validReq
	emptyHealthCheckType.Registration.HealthCheck.Type = ""
	dic := mockDic()
	dbClientMock := &mocks.DBClient{}
	dbClientMock.On("AddRegistration", validRegistrationModel).Return(validRegistrationModel, nil)
	dbClientMock.On("AddRegistration", dtos.ToRegistrationModel(duplicateServiceId.Registration)).Return(models.Registration{}, errors.NewCommonEdgeX(errors.KindDuplicateName, "duplicated", nil))
	registryMock := &mocks.Registry{}
	registryMock.On("Register", validRegistrationModel)
	dic.Update(di.ServiceConstructorMap{
		container.DBClientInterfaceName: func(get di.Get) interface{} {
			return dbClientMock
		},
		container.RegistryInterfaceName: func(get di.Get) interface{} {
			return registryMock
		},
	})

	controller := NewRegistryController(dic)
	assert.NotNil(t, controller)
	tests := []struct {
		name               string
		request            requests.AddRegistrationRequest
		expectedStatusCode int
	}{
		{"valid", validReq, http.StatusCreated},
		{"invalid - empty serviceId", emptyServiceId, http.StatusBadRequest},
		{"invalid - invalid interval format", invalidInterval, http.StatusBadRequest},
		{"invalid - empty health check type", emptyHealthCheckType, http.StatusBadRequest},
		{"invalid - duplicated serviceId", duplicateServiceId, http.StatusConflict},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			jsonData, err := json.Marshal(testCase.request)
			require.NoError(t, err)

			reader := strings.NewReader(string(jsonData))
			req, err := http.NewRequest(http.MethodPost, constants.ApiRegisterRoute, reader)
			require.NoError(t, err)

			// Act
			recorder := httptest.NewRecorder()
			handler := http.HandlerFunc(controller.Register)
			handler.ServeHTTP(recorder, req)

			// Assert
			assert.Equal(t, testCase.expectedStatusCode, recorder.Result().StatusCode, "HTTP status code not as expected")
			if testCase.expectedStatusCode == http.StatusCreated {
				registryMock.AssertNumberOfCalls(t, "Register", 1)
			}
		})
	}
}

func TestRegistryController_UpdateRegister(t *testing.T) {
	validReq := buildTestRegistrationRequest()
	validRegistrationModel := dtos.ToRegistrationModel(validReq.Registration)
	validRegistrationModel.Status = v2Models.Unknown
	notFoundServiceId := validReq
	notFoundServiceId.Registration.ServiceId = "notfound"
	notFoundServiceId.Registration.Status = v2Models.Up
	emptyServiceId := validReq
	emptyServiceId.Registration.ServiceId = ""
	invalidInterval := validReq
	invalidInterval.Registration.HealthCheck.Interval = "10t"
	emptyHealthCheckType := validReq
	emptyHealthCheckType.Registration.HealthCheck.Type = ""
	dic := mockDic()
	dbClientMock := &mocks.DBClient{}
	dbClientMock.On("UpdateRegistration", validRegistrationModel).Return(nil)
	dbClientMock.On("UpdateRegistration", dtos.ToRegistrationModel(notFoundServiceId.Registration)).Return(errors.NewCommonEdgeX(errors.KindEntityDoesNotExist, "not found", nil))
	registryMock := &mocks.Registry{}
	registryMock.On("Register", validRegistrationModel)
	registryMock.On("DeregisterByServiceId", validReq.Registration.ServiceId)
	dic.Update(di.ServiceConstructorMap{
		container.DBClientInterfaceName: func(get di.Get) interface{} {
			return dbClientMock
		},
		container.RegistryInterfaceName: func(get di.Get) interface{} {
			return registryMock
		},
	})

	controller := NewRegistryController(dic)
	assert.NotNil(t, controller)
	tests := []struct {
		name               string
		request            requests.AddRegistrationRequest
		expectedStatusCode int
	}{
		{"valid", validReq, http.StatusNoContent},
		{"invalid - empty serviceId", emptyServiceId, http.StatusBadRequest},
		{"invalid - invalid interval format", invalidInterval, http.StatusBadRequest},
		{"invalid - empty health check type", emptyHealthCheckType, http.StatusBadRequest},
		{"invalid - serviceId not exists", notFoundServiceId, http.StatusNotFound},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			jsonData, err := json.Marshal(testCase.request)
			require.NoError(t, err)

			reader := strings.NewReader(string(jsonData))
			req, err := http.NewRequest(http.MethodPut, constants.ApiRegisterRoute, reader)
			require.NoError(t, err)

			// Act
			recorder := httptest.NewRecorder()
			handler := http.HandlerFunc(controller.UpdateRegister)
			handler.ServeHTTP(recorder, req)

			// Assert
			assert.Equal(t, testCase.expectedStatusCode, recorder.Result().StatusCode, "HTTP status code not as expected")
			if testCase.expectedStatusCode == http.StatusNoContent {
				registryMock.AssertNumberOfCalls(t, "Register", 1)
			}
		})
	}
}

func TestRegistryController_Deregister(t *testing.T) {
	notFound := "notFound"
	emptyServiceId := ""
	dic := mockDic()
	dbClientMock := &mocks.DBClient{}
	dbClientMock.On("DeleteRegistrationByServiceId", testServiceId).Return(nil)
	dbClientMock.On("DeleteRegistrationByServiceId", notFound).Return(errors.NewCommonEdgeX(errors.KindEntityDoesNotExist, "not found", nil))
	registryMock := &mocks.Registry{}
	registryMock.On("DeregisterByServiceId", testServiceId)
	dic.Update(di.ServiceConstructorMap{
		container.DBClientInterfaceName: func(get di.Get) interface{} {
			return dbClientMock
		},
		container.RegistryInterfaceName: func(get di.Get) interface{} {
			return registryMock
		},
	})
	controller := NewRegistryController(dic)
	assert.NotNil(t, controller)

	tests := []struct {
		name               string
		serviceId          string
		expectedStatusCode int
	}{
		{"valid", testServiceId, http.StatusNoContent},
		{"invalid - serviceId not found", notFound, http.StatusNotFound},
		{"invalid - empty serviceId", emptyServiceId, http.StatusBadRequest},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodDelete, constants.ApiRegistrationByServiceIdRoute, http.NoBody)
			req = mux.SetURLVars(req, map[string]string{constants.ServiceId: testCase.serviceId})
			require.NoError(t, err)

			// Act
			recorder := httptest.NewRecorder()
			handler := http.HandlerFunc(controller.Deregister)
			handler.ServeHTTP(recorder, req)

			// Assert
			assert.Equal(t, recorder.Result().StatusCode, testCase.expectedStatusCode)
			if testCase.expectedStatusCode == http.StatusNoContent {
				registryMock.AssertNumberOfCalls(t, "DeregisterByServiceId", 1)

			}
		})
	}
}

func TestRegistryController_RegistrationByServiceId(t *testing.T) {
	validRegistrationModel := dtos.ToRegistrationModel(buildTestRegistrationRequest().Registration)
	notFound := "notFound"
	emptyServiceId := ""
	dic := mockDic()
	dbClientMock := &mocks.DBClient{}
	dbClientMock.On("RegistrationByServiceId", testServiceId).Return(validRegistrationModel, nil)
	dbClientMock.On("RegistrationByServiceId", notFound).Return(models.Registration{}, errors.NewCommonEdgeX(errors.KindEntityDoesNotExist, "not found", nil))
	dic.Update(di.ServiceConstructorMap{
		container.DBClientInterfaceName: func(get di.Get) interface{} {
			return dbClientMock
		},
	})
	controller := NewRegistryController(dic)
	assert.NotNil(t, controller)

	tests := []struct {
		name               string
		serviceId          string
		expectedStatusCode int
	}{
		{"valid", testServiceId, http.StatusOK},
		{"invalid - serviceId not found", notFound, http.StatusNotFound},
		{"invalid - empty serviceId", emptyServiceId, http.StatusBadRequest},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, constants.ApiRegistrationByServiceIdRoute, http.NoBody)
			req = mux.SetURLVars(req, map[string]string{constants.ServiceId: testCase.serviceId})
			require.NoError(t, err)

			// Act
			recorder := httptest.NewRecorder()
			handler := http.HandlerFunc(controller.RegistrationByServiceId)
			handler.ServeHTTP(recorder, req)

			// Assert
			if testCase.expectedStatusCode != http.StatusOK {
				var res commonDTO.BaseResponse
				err = json.Unmarshal(recorder.Body.Bytes(), &res)
				require.NoError(t, err)
				assert.Equal(t, common.ApiVersion, res.ApiVersion, "API Version not as expected")
				assert.Equal(t, testCase.expectedStatusCode, recorder.Result().StatusCode, "HTTP status code not as expected")
				assert.Equal(t, testCase.expectedStatusCode, int(res.StatusCode), "Response status code not as expected")
				assert.NotEmpty(t, res.Message, "Response message doesn't contain the error message")
			} else {
				var res responses.RegistrationResponse
				err = json.Unmarshal(recorder.Body.Bytes(), &res)
				require.NoError(t, err)
				assert.Equal(t, common.ApiVersion, res.ApiVersion, "API Version not as expected")
				assert.Equal(t, testCase.expectedStatusCode, recorder.Result().StatusCode, "HTTP status code not as expected")
				assert.Equal(t, testCase.expectedStatusCode, int(res.StatusCode), "Response status code not as expected")
				assert.Equal(t, testCase.serviceId, res.Registration.ServiceId, "serviceId not as expected")
				assert.Empty(t, res.Message, "Message should be empty when it is successful")
			}
		})
	}
}

func TestRegistryController_Registrations(t *testing.T) {
	validRegistrationModel := dtos.ToRegistrationModel(buildTestRegistrationRequest().Registration)
	dic := mockDic()
	dbClientMock := &mocks.DBClient{}
	dbClientMock.On("Registrations").Return([]models.Registration{validRegistrationModel}, nil)
	dic.Update(di.ServiceConstructorMap{
		container.DBClientInterfaceName: func(get di.Get) interface{} {
			return dbClientMock
		},
	})
	controller := NewRegistryController(dic)
	assert.NotNil(t, controller)

	req, err := http.NewRequest(http.MethodGet, constants.ApiAllRegistrationsRoute, http.NoBody)
	require.NoError(t, err)

	// Act
	recorder := httptest.NewRecorder()
	handler := http.HandlerFunc(controller.Registrations)
	handler.ServeHTTP(recorder, req)

	// Assert
	var res responses.MultiRegistrationsResponse
	err = json.Unmarshal(recorder.Body.Bytes(), &res)
	require.NoError(t, err)
	assert.Equal(t, common.ApiVersion, res.ApiVersion, "API Version not as expected")
	assert.Equal(t, http.StatusOK, recorder.Result().StatusCode, "HTTP status code not as expected")
	assert.Equal(t, uint32(1), res.TotalCount, "Total count not as expected")
	assert.Equal(t, 1, len(res.Registrations), "Device count not as expected")
	assert.Empty(t, res.Message, "Message should be empty when it is successful")
}
