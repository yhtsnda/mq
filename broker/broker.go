package broker

import (
	"os"
	"sync"

	"github.com/asim/mq/go/client"
)

var (
	Default Broker = newBroker()
)

// internal broker
type broker struct {
	options *Options

	sync.RWMutex
	topics map[string][]chan []byte
}

// Broker is the message broker
type Broker interface {
	Publish(topic string, payload []byte) error
	Subscribe(topic string) (<-chan []byte, error)
	Unsubscribe(topic string, sub <-chan []byte) error
}

func newBroker(opts ...Option) *broker {
	options := &Options{
		Client: client.New(),
	}

	for _, o := range opts {
		o(options)
	}

	return &broker{
		options: options,
		topics:  make(map[string][]chan []byte),
	}
}

func (b *broker) persist(topic string) error {
	ch, err := b.Subscribe(topic)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(topic+".mq", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
	if err != nil {
		return err
	}

	go func() {
		for b := range ch {
			f.Write(b)
			f.Write([]byte{'\n'})
		}
	}()

	return nil
}

func (b *broker) Publish(topic string, payload []byte) error {
	if b.options.Proxy {
		return b.options.Client.Publish(topic, payload)
	}

	b.RLock()
	subscribers, ok := b.topics[topic]
	b.RUnlock()
	if !ok {
		// persist?
		if !b.options.Persist {
			return nil
		}
		if err := b.persist(topic); err != nil {
			return err
		}
	}

	go func() {
		for _, subscriber := range subscribers {
			select {
			case subscriber <- payload:
			default:
			}
		}
	}()

	return nil
}

func (b *broker) Subscribe(topic string) (<-chan []byte, error) {
	if b.options.Proxy {
		return b.options.Client.Subscribe(topic)
	}

	ch := make(chan []byte, 100)
	b.Lock()
	b.topics[topic] = append(b.topics[topic], ch)
	b.Unlock()
	return ch, nil
}

func (b *broker) Unsubscribe(topic string, sub <-chan []byte) error {
	if b.options.Proxy {
		return b.options.Client.Unsubscribe(sub)
	}

	b.RLock()
	subscribers, ok := b.topics[topic]
	b.RUnlock()

	if !ok {
		return nil
	}

	var subs []chan []byte
	for _, subscriber := range subscribers {
		if subscriber == sub {
			continue
		}
		subs = append(subs, subscriber)
	}

	b.Lock()
	b.topics[topic] = subs
	b.Unlock()
	return nil
}

func Publish(topic string, payload []byte) error {
	return Default.Publish(topic, payload)
}

func Subscribe(topic string) (<-chan []byte, error) {
	return Default.Subscribe(topic)
}

func Unsubscribe(topic string, sub <-chan []byte) error {
	return Default.Unsubscribe(topic, sub)
}

func New(opts ...Option) *broker {
	return newBroker(opts...)
}
