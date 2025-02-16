/*
 Copyright 2021 The CloudEvents Authors
 SPDX-License-Identifier: Apache-2.0
*/

package kafka_sarama_binding

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
	"github.com/cloudevents/sdk-go/v2/binding"
	"github.com/cloudevents/sdk-go/v2/event"
	bindings "github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/cloudevents/sdk-go/v2/protocol/test"

	. "github.com/cloudevents/sdk-go/v2/binding/test"
	. "github.com/cloudevents/sdk-go/v2/test"
)

const (
	TEST_GROUP_ID   = "test_group_id"
	KAFKA_OFFSET    = "kafkaoffset"
	KAFKA_PARTITION = "kafkapartition"
	KAFKA_TOPIC     = "kafkatopic"
)

func TestSendStructuredMessageToStructured(t *testing.T) {
	close, s, r, _ := testSenderReceiver(t)
	defer close()
	EachEvent(t, Events(), func(t *testing.T, eventIn event.Event) {
		eventIn = ConvertEventExtensionsToString(t, eventIn)

		in := MustCreateMockStructuredMessage(t, eventIn)
		test.SendReceive(t, binding.WithPreferredEventEncoding(context.TODO(), binding.EncodingStructured), in, s, r, func(out binding.Message) {
			eventOut := MustToEvent(t, context.Background(), out)
			assert.Equal(t, binding.EncodingStructured, out.ReadEncoding())
			AssertEventEquals(t, eventIn, ConvertEventExtensionsToString(t, eventOut))
		})
	})
}

func TestSendBinaryMessageToBinary(t *testing.T) {
	close, s, r, topicName := testSenderReceiver(t)
	defer close()
	EachEvent(t, Events(), func(t *testing.T, eventIn event.Event) {
		eventIn = ConvertEventExtensionsToString(t, eventIn)

		in := MustCreateMockBinaryMessage(eventIn)
		test.SendReceive(t, binding.WithPreferredEventEncoding(context.TODO(), binding.EncodingBinary), in, s, r, func(out binding.Message) {
			assert.Equal(t, binding.EncodingBinary, out.ReadEncoding())
			AllOf(
				HasExactlyAttributesEqualTo(eventIn.Context),
				HasData(eventIn.Data()),
				HasExtensionKeys([]string{KAFKA_OFFSET, KAFKA_PARTITION}),
				HasExtension(KAFKA_TOPIC, topicName),
			)
		})
	})
}

// To start a local environment for testing:
// docker run --rm --net=host -e ADV_HOST=localhost -e SAMPLEDATA=0 lensesio/fast-data-dev
func testClient(t testing.TB) sarama.Client {
	t.Helper()
	s := os.Getenv("TEST_KAFKA_BOOTSTRAP_SERVER")
	if s == "" {
		s = "localhost:9092"
	}

	config := sarama.NewConfig()
	config.Version = sarama.V2_0_0_0
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true
	config.Consumer.Offsets.Initial = sarama.OffsetOldest
	client, err := sarama.NewClient(strings.Split(s, ","), config)
	if err != nil {
		t.Skipf("Cannot create sarama client to servers [%s]: %v", s, err)
	}

	return client
}

func testSenderReceiver(t testing.TB) (func(), bindings.Sender, bindings.Receiver, string) {
	client := testClient(t)

	topicName := "test-ce-client-" + uuid.New().String()
	p, err := kafka_sarama.NewProtocolFromClient(client, topicName, topicName, kafka_sarama.WithReceiverGroupId(TEST_GROUP_ID))
	require.NoError(t, err)
	require.NotNil(t, p)

	go func() {
		require.NoError(t, p.OpenInbound(context.TODO()))
	}()

	// Not perfect but we need to give OpenInbound() as chance to start
	// as it's a race condition. I couldn't find something on 'p' to wait for
	time.Sleep(15 * time.Second)

	return func() {
		require.NoError(t, p.Close(context.TODO()))
		require.NoError(t, client.Close())
	}, p, p, topicName
}

func BenchmarkSendReceive(b *testing.B) {
	c, s, r, _ := testSenderReceiver(b)
	defer c() // Cleanup
	test.BenchmarkSendReceive(b, s, r)
}
