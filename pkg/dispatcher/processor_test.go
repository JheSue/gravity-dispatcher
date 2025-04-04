package dispatcher

import (
	"errors"
	"sync"
	"testing"

	"github.com/BrobridgeOrg/gravity-dispatcher/pkg/dispatcher/rule_manager"
	product_sdk "github.com/BrobridgeOrg/gravity-sdk/v2/product"
	record_type "github.com/BrobridgeOrg/gravity-sdk/v2/types/record"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func CreateTestRule() *rule_manager.Rule {

	r := rule_manager.NewRule(product_sdk.NewRule())
	r.Event = "dataCreated"
	r.Product = "TestDataProduct"
	r.PrimaryKey = []string{
		"id",
	}

	schemaRaw := `{
	"id": { "type": "int" },
	"name": { "type": "string" },
	"gender": { "type": "string" },
	"nested": {
		"type": "map",
		"fields": {
			"nested_id": { "type": "string" }
		}
	},
	"tags": {
		"type": "array",
		"subtype": "string"
	}
}`

	var schemaConfig map[string]interface{}
	err := json.Unmarshal([]byte(schemaRaw), &schemaConfig)
	if err != nil {
		panic(err)
	}

	r.SchemaConfig = schemaConfig

	return r
}

func CreateTestMessage() *Message {

	// Prparing rule
	testRuleManager := rule_manager.NewRuleManager()
	r := CreateTestRule()
	testRuleManager.AddRule(r)

	msg := NewMessage()
	msg.Rule = r

	return msg
}

func GetFieldValue(r *record_type.Record, fieldName string) (interface{}, error) {

	for _, field := range r.Payload.Map.Fields {
		if field.Name == fieldName {
			return record_type.GetValueData(field.Value), nil
		}
	}

	return nil, errors.New("Field not found")
}

func TestProcessorOutput(t *testing.T) {

	logger = zap.NewNop()

	done := make(chan struct{})

	p := NewProcessor(
		WithOutputHandler(func(msg *Message) {
			assert.Equal(t, "dataCreated", msg.ProductEvent.EventName)
			assert.Equal(t, "TestDataProduct", msg.ProductEvent.Table)

			r, err := msg.ProductEvent.GetContent()
			assert.Equal(t, nil, err)

			for _, field := range r.Payload.Map.Fields {
				switch field.Name {
				case "id":
					assert.Equal(t, int64(101), record_type.GetValueData(field.Value))
				case "name":
					assert.Equal(t, "fred", record_type.GetValueData(field.Value))
				}
			}

			done <- struct{}{}
		}),
	)

	testData := MessageRawData{
		Event:      "dataCreated",
		RawPayload: []byte(`{"id":101,"name":"fred"}`),
	}

	// Preparing message with raw data
	msg := CreateTestMessage()
	raw, _ := json.Marshal(testData)
	msg.Raw = raw

	p.Push(msg)

	<-done
}

func TestProcessor_UpdateNestedFields(t *testing.T) {

	logger = zap.NewNop()

	done := make(chan struct{})

	p := NewProcessor(
		WithOutputHandler(func(msg *Message) {
			assert.Equal(t, "dataCreated", msg.ProductEvent.EventName)
			assert.Equal(t, "TestDataProduct", msg.ProductEvent.Table)

			r, err := msg.ProductEvent.GetContent()
			if !assert.Nil(t, err) {
				return
			}

			if v, err := GetFieldValue(r, "$removedFields"); assert.Nil(t, err) {
				for _, ele := range v.([]interface{}) {
					assert.Equal(t, "id", ele.(string))
				}
			}

			if v, err := GetFieldValue(r, "nested.nested_id"); assert.Nil(t, err) {
				assert.Equal(t, "hello", v)
			}

			if v, err := GetFieldValue(r, "tags.0"); assert.Nil(t, err) {
				assert.Equal(t, "new_tag1", v)
			}

			done <- struct{}{}
		}),
	)

	testData := MessageRawData{
		Event: "dataCreated",
		RawPayload: []byte(`{
	"$removedFields": ["id"],
	"nested.nested_id": "hello",
	"tags.0": "new_tag1"
}`),
	}

	// Preparing message with raw data
	msg := CreateTestMessage()
	raw, _ := json.Marshal(testData)
	msg.Raw = raw

	p.Push(msg)

	<-done
}

func TestProcessorOutputsWithMultipleInputs(t *testing.T) {

	logger = zap.NewNop()

	var wg sync.WaitGroup
	count := int64(0)

	p := NewProcessor(
		WithOutputHandler(func(msg *Message) {
			assert.Equal(t, "dataCreated", msg.ProductEvent.EventName)
			assert.Equal(t, "TestDataProduct", msg.ProductEvent.Table)

			count++

			r, err := msg.ProductEvent.GetContent()
			assert.Equal(t, nil, err)

			for _, field := range r.Payload.Map.Fields {
				switch field.Name {
				case "id":
					assert.Equal(t, count, record_type.GetValueData(field.Value))
				case "name":
					assert.Equal(t, "test", record_type.GetValueData(field.Value))
				}
			}

			wg.Done()
		}),
	)

	num := 1000
	wg.Add(num)
	for i := 1; i <= num; i++ {

		rawPayload := map[string]interface{}{
			"id":   i,
			"name": "test",
		}

		payload, _ := json.Marshal(rawPayload)

		testData := MessageRawData{
			Event:      "dataCreated",
			RawPayload: payload,
		}

		// Preparing message with raw data
		msg := CreateTestMessage()
		raw, _ := json.Marshal(testData)
		msg.Raw = raw

		p.Push(msg)
	}

	wg.Wait()
}

func TestProcessorOutputsWithVariousInputs(t *testing.T) {

	logger = zap.NewNop()

	var wg sync.WaitGroup
	count := int64(0)

	payloads := []map[string]interface{}{
		{
			"id":   int64(1),
			"name": "fred",
		},
		{
			"id":     int64(2),
			"gender": "male",
		},
		{
			"id":   int64(3),
			"name": "stacy",
		},
		{
			"id":     int64(4),
			"gender": "male",
		},
		{
			"id":   int64(5),
			"name": "stacy",
		},
		{
			"id":   int64(6),
			"name": "fred",
		},
		{
			"id":     int64(7),
			"gender": "female",
		},
		{
			"id":   int64(6),
			"name": "fred",
		},
	}

	p := NewProcessor(
		WithOutputHandler(func(msg *Message) {
			assert.Equal(t, "dataCreated", msg.ProductEvent.EventName)
			assert.Equal(t, "TestDataProduct", msg.ProductEvent.Table)

			payload := payloads[int(count)]

			count++

			r, err := msg.ProductEvent.GetContent()
			assert.Equal(t, nil, err)
			assert.Equal(t, len(payload), len(r.Payload.Map.Fields))

			for k, v := range payload {
				var targetField *record_type.Field = nil
				for _, field := range r.Payload.Map.Fields {
					if field.Name == k {
						targetField = field
						break
					}
				}

				assert.NotNil(t, targetField)
				assert.Equal(t, v, record_type.GetValueData(targetField.Value))
			}

			wg.Done()
		}),
	)

	wg.Add(len(payloads))
	for _, pl := range payloads {

		payload, _ := json.Marshal(pl)

		testData := MessageRawData{
			Event:      "dataCreated",
			RawPayload: payload,
		}

		// Preparing message with raw data
		msg := CreateTestMessage()
		raw, _ := json.Marshal(testData)
		msg.Raw = raw

		p.Push(msg)
	}

	wg.Wait()
}
