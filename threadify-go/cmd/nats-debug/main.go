package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func main() {
	server := flag.String("server", "nats://localhost:4222", "NATS server URL")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		usage()
		os.Exit(1)
	}

	nc, err := nats.Connect(*server)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		log.Fatalf("jetstream: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch args[0] {
	case "stream":
		checkStream(ctx, js)
	case "consumer":
		checkConsumer(ctx, js)
	case "dump":
		n := 5
		if len(args) > 1 {
			if v, err := strconv.Atoi(args[1]); err == nil && v > 0 {
				n = v
			}
		}
		dumpMessages(ctx, js, n)
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("Usage: nats-debug [flags] <command> [args]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  stream              Show thread_metadata stream info")
	fmt.Println("  consumer            Show archiver consumer lag/info")
	fmt.Println("  dump [count]        Dump last N messages (default 5)")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -server string     NATS server URL (default \"nats://localhost:4222\")")
}

func checkStream(ctx context.Context, js jetstream.JetStream) {
	stream, err := js.Stream(ctx, "thread_metadata")
	if err != nil {
		log.Fatalf("stream: %v", err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		log.Fatalf("stream info: %v", err)
	}

	fmt.Printf("Stream:   %s\n", info.Config.Name)
	fmt.Printf("Subjects: %v\n", info.Config.Subjects)
	fmt.Printf("Messages: %d\n", info.State.Msgs)
}

func checkConsumer(ctx context.Context, js jetstream.JetStream) {
	cons, err := js.Consumer(ctx, "thread_metadata", "archiver-thread_metadata")
	if err != nil {
		log.Fatalf("consumer: %v", err)
	}
	info, err := cons.Info(ctx)
	if err != nil {
		log.Fatalf("consumer info: %v", err)
	}

	fmt.Printf("Consumer:       %s\n", info.Config.Name)
	fmt.Printf("Filter Subject: %s\n", info.Config.FilterSubject)
	fmt.Printf("NumAckPending:  %d\n", info.NumAckPending)
	fmt.Printf("NumPending:     %d\n", info.NumPending)
	fmt.Printf("Delivered:      %d\n", info.Delivered.Stream)
	fmt.Printf("AckFloor:       %d\n", info.AckFloor.Stream)
}

func dumpMessages(ctx context.Context, js jetstream.JetStream, count int) {
	stream, err := js.Stream(ctx, "thread_metadata")
	if err != nil {
		log.Fatalf("stream: %v", err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		log.Fatalf("stream info: %v", err)
	}

	startSeq := info.State.LastSeq - uint64(count) + 1
	if startSeq < 1 {
		startSeq = 1
	}

	cons, err := js.OrderedConsumer(ctx, "thread_metadata", jetstream.OrderedConsumerConfig{
		DeliverPolicy: jetstream.DeliverByStartSequencePolicy,
		OptStartSeq:   startSeq,
	})
	if err != nil {
		log.Fatalf("ordered consumer: %v", err)
	}

	msgContext, err := cons.Messages()
	if err != nil {
		log.Fatalf("messages: %v", err)
	}
	defer msgContext.Stop()

	for i := 0; i < count; i++ {
		msg, err := msgContext.Next()
		if err != nil {
			break
		}
		fmt.Printf("Message (%d/%d):\n%s\n\n", i+1, count, string(msg.Data()))
	}
}
