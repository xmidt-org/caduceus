package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/ancla"
	"github.com/xmidt-org/argus/chrysom"
	"github.com/xmidt-org/argus/model"
	"github.com/xmidt-org/sallust"
	"go.uber.org/zap"
)

func TestNewHelperService(t *testing.T) {
	tcs := []struct {
		desc        string
		config      ancla.Config
		getLogger   func(context.Context) *zap.Logger
		expectedErr bool
	}{
		{
			desc: "Success Case",
			config: ancla.Config{
				BasicClientConfig: chrysom.BasicClientConfig{
					Address: "test",
					Bucket:  "test",
				},
			},
		},
		{
			desc:        "Chrysom Basic Client Creation Failure",
			expectedErr: true,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			assert := assert.New(t)
			_, err := NewHelperService(tc.config, tc.getLogger)
			if tc.expectedErr {
				assert.NotNil(err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestStartHelperListener(t *testing.T) {
	mockServiceConfig := ancla.Config{
		BasicClientConfig: chrysom.BasicClientConfig{
			Address: "test",
			Bucket:  "test",
		},
	}
	mockService, _ := NewHelperService(mockServiceConfig, nil)
	tcs := []struct {
		desc           string
		serviceConfig  ancla.Config
		listenerConfig HelperListenerConfig
		svc            helperservice
		expectedErr    bool
	}{
		{
			desc: "Success Case",
			svc:  *mockService,
			listenerConfig: HelperListenerConfig{
				Config: chrysom.ListenerClientConfig{},
			},
		},
		{
			desc:        "Chrysom Listener Client Creation Failure",
			expectedErr: true,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			assert := assert.New(t)
			_, err := tc.svc.StartHelperListener(tc.listenerConfig, nil)
			if tc.expectedErr {
				assert.NotNil(err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestAdd(t *testing.T) {
	type pushItemResults struct {
		result chrysom.PushResult
		err    error
	}
	type testCase struct {
		Description     string
		Owner           string
		PushItemResults pushItemResults
		ExpectedErr     error
	}

	tcs := []testCase{
		{
			Description: "PushItem fails",
			PushItemResults: pushItemResults{
				err: errors.New("push item failed"),
			},
			ExpectedErr: errFailedWebhookPush,
		},
		{
			Description: "Unknown push result",
			PushItemResults: pushItemResults{
				result: chrysom.UnknownPushResult,
			},
			ExpectedErr: errNonSuccessPushResult,
		},
		{
			Description: "Item created",
			PushItemResults: pushItemResults{
				result: chrysom.CreatedPushResult,
			},
		},
		{
			Description: "Item update",
			PushItemResults: pushItemResults{
				result: chrysom.UpdatedPushResult,
			},
		},
	}

	inputWebhook := getTestInternalWebhooks()[0]

	for _, tc := range tcs {
		t.Run(tc.Description, func(t *testing.T) {
			assert := assert.New(t)
			m := new(mockPushReader)
			svc := helperservice{
				logger: sallust.Default(),
				config: ancla.Config{},
				argus:  m,
				now:    time.Now,
			}
			// nolint:typecheck
			m.On("PushItem", context.TODO(), tc.Owner, mock.Anything).Return(tc.PushItemResults.result, tc.PushItemResults.err)
			err := svc.Add(context.TODO(), tc.Owner, inputWebhook)
			if tc.ExpectedErr != nil {
				assert.True(errors.Is(err, tc.ExpectedErr))
			}
			// nolint:typecheck
			m.AssertExpectations(t)
		})
	}
}

func TestAllInternalWebhooks(t *testing.T) {
	type testCase struct {
		Description              string
		GetItemsResp             chrysom.Items
		GetItemsErr              error
		ExpectedInternalWebhooks []ancla.InternalWebhook
		ExpectedErr              error
	}

	tcs := []testCase{
		{
			Description: "Fetching argus webhooks fails",
			GetItemsErr: errors.New("db failed"),
			ExpectedErr: errFailedWebhooksFetch,
		},
		{
			Description:              "Webhooks fetch success",
			GetItemsResp:             getTestItems(),
			ExpectedInternalWebhooks: getTestInternalWebhooks(),
		},
	}

	for _, tc := range tcs {
		t.Run(tc.Description, func(t *testing.T) {
			assert := assert.New(t)
			m := new(mockPushReader)

			svc := helperservice{
				argus:  m,
				logger: sallust.Default(),
				config: ancla.Config{},
			}
			// nolint:typecheck
			m.On("GetItems", context.TODO(), "").Return(tc.GetItemsResp, tc.GetItemsErr)
			iws, err := svc.GetAll(context.TODO())

			if tc.ExpectedErr != nil {
				assert.True(errors.Is(err, tc.ExpectedErr))
				assert.Empty(iws)
			} else {
				assert.EqualValues(tc.ExpectedInternalWebhooks, iws)
			}

			// nolint:typecheck
			m.AssertExpectations(t)
		})
	}
}

func TestNewJWTAcquireParser(t *testing.T) {
	tcs := []struct {
		Description string
		ParserType  jwtAcquireParserType
		ShouldFail  bool
	}{
		{
			Description: "Default",
		},
		{
			Description: "Invalid type",
			ParserType:  "advanced",
			ShouldFail:  true,
		},
		{
			Description: "Simple",
			ParserType:  simpleType,
		},
		{
			Description: "Raw",
			ParserType:  rawType,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.Description, func(t *testing.T) {
			assert := assert.New(t)
			p, err := newJWTAcquireParser(tc.ParserType)
			if tc.ShouldFail {
				assert.NotNil(err)
				assert.Nil(p.expiration)
				assert.Nil(p.token)
			} else {
				assert.Nil(err)
				if tc.ParserType == rawType {
					assert.NotNil(p.expiration)
					assert.NotNil(p.token)
				}
			}
		})
	}
}

func TestRawTokenParser(t *testing.T) {
	assert := assert.New(t)
	payload := []byte("eyJhbGciOiJSUzI1NiIsImtpZCI6ImRldmVsb3BtZW50IiwidHlwIjoiSldUIn0.eyJhbGxvd2VkUmVzb3VyY2VzIjp7ImFsbG93ZWRQYXJ0bmVycyI6WyJjb21jYXN0Il19LCJhdWQiOiJYTWlEVCIsImNhcGFiaWxpdGllcyI6WyJ4MTppc3N1ZXI6dGVzdDouKjphbGwiLCJ4MTppc3N1ZXI6dWk6YWxsIl0sImV4cCI6MTYyMjE1Nzk4MSwiaWF0IjoxNjIyMDcxNTgxLCJpc3MiOiJkZXZlbG9wbWVudCIsImp0aSI6ImN4ZmkybTZDWnJjaFNoZ1Nzdi1EM3ciLCJuYmYiOjE2MjIwNzE1NjYsInBhcnRuZXItaWQiOiJjb21jYXN0Iiwic3ViIjoiY2xpZW50LXN1cHBsaWVkIiwidHJ1c3QiOjEwMDB9.7QzRWJgxGs1cEZunMOewYCnEDiq2CTDh5R5F47PYhkMVb2KxSf06PRRGN-rQSWPhhBbev1fGgu63mr3yp_VDmdVvHR2oYiKyxP2skJTSzfQmiRyLMYY5LcLn3BObyQxU8EnLhnqGIjpORW0L5Dd4QsaZmXRnkC73yGnJx4XCx0I")
	token, err := rawTokenParser(payload)
	assert.Equal(string(payload), token)
	assert.Nil(err)
}

func TestRawExpirationParser(t *testing.T) {
	tcs := []struct {
		Description  string
		Payload      []byte
		ShouldFail   bool
		ExpectedTime time.Time
	}{
		{
			Description: "Not a JWT",
			Payload:     []byte("xyz==abcNotAJWT"),
			ShouldFail:  true,
		},
		{
			Description:  "A jwt",
			Payload:      []byte("eyJhbGciOiJSUzI1NiIsImtpZCI6ImRldmVsb3BtZW50IiwidHlwIjoiSldUIn0.eyJhbGxvd2VkUmVzb3VyY2VzIjp7ImFsbG93ZWRQYXJ0bmVycyI6WyJjb21jYXN0Il19LCJhdWQiOiJYTWlEVCIsImNhcGFiaWxpdGllcyI6WyJ4MTppc3N1ZXI6dGVzdDouKjphbGwiLCJ4MTppc3N1ZXI6dWk6YWxsIl0sImV4cCI6MTYyMjE1Nzk4MSwiaWF0IjoxNjIyMDcxNTgxLCJpc3MiOiJkZXZlbG9wbWVudCIsImp0aSI6ImN4ZmkybTZDWnJjaFNoZ1Nzdi1EM3ciLCJuYmYiOjE2MjIwNzE1NjYsInBhcnRuZXItaWQiOiJjb21jYXN0Iiwic3ViIjoiY2xpZW50LXN1cHBsaWVkIiwidHJ1c3QiOjEwMDB9.7QzRWJgxGs1cEZunMOewYCnEDiq2CTDh5R5F47PYhkMVb2KxSf06PRRGN-rQSWPhhBbev1fGgu63mr3yp_VDmdVvHR2oYiKyxP2skJTSzfQmiRyLMYY5LcLn3BObyQxU8EnLhnqGIjpORW0L5Dd4QsaZmXRnkC73yGnJx4XCx0I"),
			ExpectedTime: time.Unix(1622157981, 0),
		},
	}

	for _, tc := range tcs {
		assert := assert.New(t)
		exp, err := rawTokenExpirationParser(tc.Payload)
		if tc.ShouldFail {
			assert.NotNil(err)
			assert.Empty(exp)
		} else {
			assert.Nil(err)
			assert.Equal(tc.ExpectedTime, exp)
		}
	}
}

func TestWebhookListSizeWatch(t *testing.T) {
	require := require.New(t)
	gauge := new(mockGauge)
	watch := webhookListSizeWatch(gauge)
	require.NotNil(watch)
	// nolint:typecheck
	gauge.On("Set", float64(2))
	watch.Update([]ancla.InternalWebhook{{}, {}})
	// nolint:typecheck
	gauge.AssertExpectations(t)
}

func getTestInternalWebhooks() []ancla.InternalWebhook {
	refTime := getRefTime()
	return []ancla.InternalWebhook{
		{
			Webhook: ancla.Webhook{
				Address: "http://original-requester.example.net",
				Config: ancla.DeliveryConfig{
					URL:         "http://deliver-here-0.example.net",
					ContentType: "application/json",
					Secret:      "superSecretXYZ",
				},
				Events: []string{"online"},
				Matcher: ancla.MetadataMatcherConfig{
					DeviceID: []string{"mac:aabbccddee.*"},
				},
				FailureURL: "http://contact-here-when-fails.example.net",
				Duration:   10 * time.Second,
				Until:      refTime.Add(10 * time.Second),
			},
			PartnerIDs: []string{"comcast"},
		},
		{
			Webhook: ancla.Webhook{
				Address: "http://original-requester.example.net",
				Config: ancla.DeliveryConfig{
					ContentType: "application/json",
					URL:         "http://deliver-here-1.example.net",
					Secret:      "doNotShare:e=mc^2",
				},
				Events: []string{"online"},
				Matcher: ancla.MetadataMatcherConfig{
					DeviceID: []string{"mac:aabbccddee.*"},
				},

				FailureURL: "http://contact-here-when-fails.example.net",
				Duration:   20 * time.Second,
				Until:      refTime.Add(20 * time.Second),
			},
			PartnerIDs: []string{},
		},
	}
}

func getTestItems() chrysom.Items {
	var (
		firstItemExpiresInSecs  int64 = 10
		secondItemExpiresInSecs int64 = 20
	)
	return chrysom.Items{
		model.Item{
			ID: "b3bbc3467366959e0aba3c33588a08c599f68a740fabf4aa348463d3dc7dcfe8",
			Data: map[string]interface{}{
				"Webhook": map[string]interface{}{
					"registered_from_address": "http://original-requester.example.net",
					"config": map[string]interface{}{
						"url":          "http://deliver-here-0.example.net",
						"content_type": "application/json",
						"secret":       "superSecretXYZ",
					},
					"events": []interface{}{"online"},
					"matcher": map[string]interface{}{
						"device_id": []interface{}{"mac:aabbccddee.*"},
					},
					"failure_url": "http://contact-here-when-fails.example.net",
					"duration":    float64((10 * time.Second).Nanoseconds()),
					"until":       "2021-01-02T15:04:10Z",
				},
				"PartnerIDs": []interface{}{"comcast"},
			},

			TTL: &firstItemExpiresInSecs,
		},
		model.Item{
			ID: "c97b4d17f7eb406720a778f73eecf419438659091039a312bebba4570e80a778",
			Data: map[string]interface{}{
				"webhook": map[string]interface{}{
					"registered_from_address": "http://original-requester.example.net",
					"config": map[string]interface{}{
						"url":          "http://deliver-here-1.example.net",
						"content_type": "application/json",
						"secret":       "doNotShare:e=mc^2",
					},
					"events": []interface{}{"online"},
					"matcher": map[string]interface{}{
						"device_id": []interface{}{"mac:aabbccddee.*"},
					},
					"failure_url": "http://contact-here-when-fails.example.net",
					"duration":    float64((20 * time.Second).Nanoseconds()),
					"until":       "2021-01-02T15:04:20Z",
				},
				"partnerids": []string{},
			},
			TTL: &secondItemExpiresInSecs,
		},
	}
}

func getRefTime() time.Time {
	refTime, err := time.Parse(time.RFC3339, "2021-01-02T15:04:00Z")
	if err != nil {
		panic(err)
	}
	return refTime
}

type mockPushReader struct {
	mock.Mock
}

func (m *mockPushReader) GetItems(ctx context.Context, owner string) (chrysom.Items, error) {
	// nolint:typecheck
	args := m.Called(ctx, owner)
	return args.Get(0).(chrysom.Items), args.Error(1)
}

func (m *mockPushReader) Start(ctx context.Context) error {
	// nolint:typecheck
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockPushReader) Stop(ctx context.Context) error {
	// nolint:typecheck
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockPushReader) PushItem(ctx context.Context, owner string, item model.Item) (chrysom.PushResult, error) {
	// nolint:typecheck
	args := m.Called(ctx, owner, item)
	return args.Get(0).(chrysom.PushResult), args.Error(1)
}

func (m *mockPushReader) RemoveItem(ctx context.Context, id, owner string) (model.Item, error) {
	// nolint:typecheck
	args := m.Called(ctx, id, owner)
	return args.Get(0).(model.Item), args.Error(0)
}
