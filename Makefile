.PHONY: test test-unit test-integration vet lint tidy

test:
	go test ./...

test-unit:
	go test ./pkg/openwechat -run 'TestIsNetworkError|TestFormatEmoji|TestMessage_FriendAddMessageContent|ExampleMessageType_output'

test-integration:
	OPENWECHAT_INTEGRATION=1 go test ./pkg/openwechat

vet:
	go vet ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy
