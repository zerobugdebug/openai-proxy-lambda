package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/apigatewaymanagementapi"
	"github.com/sashabaranov/go-openai"

)

const (
	defaultModel          = "gpt-3.5-turbo"
	statusCodeOK          = 200
	statusCodeBadRequest  = 400
	statusCodeServerError = 500
	connectRouteKey       = "$connect"
	disconnectRouteKey    = "$disconnect"
	responseTypeInt       = "int"
	responseTypeString    = "string"
	responseTypeFull      = "full"
	responseTypeStream    = "stream"
	endStreamMessage      = "<END>"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type Request struct {
	PromptTemplate string        `json:"prompt_template"`
	Messages       []chatMessage `json:"messages"`
	ResponseType   string        `json:"response_type"`
}

type openAIRequest struct {
	request          Request
	apiGatewayClient *apigatewaymanagementapi.ApiGatewayManagementApi
	ConnectionId     string
}

type WebsocketHandler struct {
	apiGatewayClient *apigatewaymanagementapi.ApiGatewayManagementApi
	apiGatewayStage  string
}

type Config struct {
	OpenAIKey          string
	OpenAIModel        string
	APIGatewayEndpoint string
}

var config Config // Global configuration variable

// getConfusables returns a read-only map of confusable characters to their ASCII replacements to imitate const map.
func getConfusables() map[rune]rune {
	return map[rune]rune{
		'“': '"',
		'”': '"',
		'‘': '\'',
		'’': '\'',
		'΄': '\'',
	}
}

// Replace confusable UTF-8 characters in s with their ASCII replacements.
func replaceConfusables(s string) string {
	confusables := getConfusables()
	var builder strings.Builder
	for _, ch := range s {
		if replacement, ok := confusables[ch]; ok {
			builder.WriteRune(replacement)
		} else {
			builder.WriteRune(ch)
		}
	}
	return builder.String()
}

// init is called to load configuration from environment variables
func init() {
	var err error
	config, err = loadConfig()
	if err != nil {
		fmt.Printf("Failed to load configuration: %v", err)
		os.Exit(1)
	}
}

func main() {
	lambda.Start(Handler)
}

// loadConfig loads configuration from environment variables
func loadConfig() (Config, error) {
	cfg := Config{
		OpenAIKey:          os.Getenv("OPENAI_API_KEY"),
		OpenAIModel:        os.Getenv("OPENAI_MODEL"),
		APIGatewayEndpoint: os.Getenv("API_GW_ENDPOINT"),
	}

	if cfg.OpenAIKey == "" {
		return cfg, fmt.Errorf("OpenAI API key not found in environment variable OPENAI_API_KEY")
	}

	if cfg.OpenAIModel == "" {
		cfg.OpenAIModel = defaultModel
	}

	if cfg.APIGatewayEndpoint == "" {
		return cfg, fmt.Errorf("API Gateway Endpoint not found in environment variable API_GW_ENDPOINT")
	}

	return cfg, nil
}

// Handler is the main handler for AWS Lambda functions
func Handler(ctx context.Context, request events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {

	/* 	fmt.Printf("request.Resource: %v\n", request.Resource)
	   	fmt.Printf("request.Path: %v\n", request.Path)
	   	fmt.Printf("request.HTTPMethod: %v\n", request.HTTPMethod)
	   	fmt.Printf("request.Body: %v\n", request.Body)
	   	fmt.Printf("request.RequestContext: %v\n", request.RequestContext)
	   	fmt.Printf("request.RequestContext.RouteKey: %v\n", request.RequestContext.RouteKey) */

	routeKey := request.RequestContext.RouteKey
	switch routeKey {
	case connectRouteKey, disconnectRouteKey:
		return handleConnection(routeKey)
	default:
		return handleRequest(request)
	}
}

// handleConnection handles connection and disconnection events
func handleConnection(routeKey string) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse{StatusCode: statusCodeOK}, nil
}

// handleRequest handles requests other than connection/disconnection
func handleRequest(request events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {
	reqBody, err := parseRequestBody(request.Body)
	if err != nil {
		return errorResponse(fmt.Sprintf("Error parsing request JSON: %s", err), statusCodeBadRequest)
	}

	apiGatewayClient := getAPIGatewayClient()
	openAIReq := createOpenAIRequest(reqBody, apiGatewayClient, request.RequestContext.ConnectionID)

	var handlerFunc func(openAIRequest) error
	switch reqBody.ResponseType {
	case "int":
		handlerFunc = getIntOpenAIResponse
	case "string":
		handlerFunc = getStringOpenAIResponse
	case "full":
		handlerFunc = getFullOpenAIResponse
	case "stream":
		handlerFunc = getStreamOpenAIResponse
	default:
		return errorResponse(fmt.Sprintf("Incorrect response type: %s", reqBody.ResponseType), statusCodeServerError)
	}

	if err := handlerFunc(openAIReq); err != nil {
		return errorResponse(fmt.Sprintf("Error handling request: %s", err), statusCodeServerError)
	}

	return events.APIGatewayProxyResponse{StatusCode: statusCodeOK}, nil
}

// parseRequestBody parses the request body from JSON to Request struct
func parseRequestBody(body string) (Request, error) {
	var reqBody Request
	err := json.Unmarshal([]byte(body), &reqBody)
	return reqBody, err
}

// errorResponse creates an error response with a specified message and status code
func errorResponse(message string, statusCode int) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse{
		Body:       message,
		StatusCode: statusCode,
	}, nil
}

// getAPIGatewayClient initializes and returns an API Gateway client
func getAPIGatewayClient() *apigatewaymanagementapi.ApiGatewayManagementApi {
	apiEndpoint := config.APIGatewayEndpoint
	return apigatewaymanagementapi.New(session.Must(session.NewSession()), aws.NewConfig().WithEndpoint(apiEndpoint))
}

// createOpenAIRequest creates an OpenAIRequest object from the given input
func createOpenAIRequest(reqBody Request, apiGatewayClient *apigatewaymanagementapi.ApiGatewayManagementApi, connectionID string) openAIRequest {
	return openAIRequest{
		request:          reqBody,
		apiGatewayClient: apiGatewayClient,
		ConnectionId:     connectionID,
	}
}

// isValidModel checks if the specified model ID is valid
func isValidModel(models []openai.Model, id string) bool {
	for _, model := range models {
		if model.ID == id {
			return true
		}
	}
	return false
}

// getOpenAIClient initializes and returns an OpenAI client
func getOpenAIClient() *openai.Client {
	return openai.NewClient(config.OpenAIKey)
}

// getModel gets the OpenAI model ID either from environment variables or defaults
func getModel() (string, error) {

	// Get the value of the "OPENAI_MODEL" environment variable
	model := config.OpenAIModel
	// Check if the model value is empty
	if model == "" {
		// If the model value is empty, set it to the default model
		return defaultModel, nil
	}
	// Otherwise, retrieve a list of available models
	client := getOpenAIClient()
	availableModels, err := client.ListModels(context.Background())
	if err != nil {
		// Print an error message and set the model to the default model
		fmt.Printf("Error getting list of available models: %s\n Defaulting to %s", err, defaultModel)
		return defaultModel, nil
	}
	// Check if the provided model is valid
	if !isValidModel(availableModels.Models, model) {
		// If it's not a valid model, print a message and set the model to the default model
		fmt.Printf("Model %s is not a valid model\n Defaulting to %s", model, defaultModel)
		return defaultModel, nil
	}
	return model, nil
}

// initOpenAIRequest initializes an OpenAI request and sends it to OpenAI
func initOpenAIRequest(promptEnvVariable string, chatMessages []chatMessage) (openai.ChatCompletionResponse, error) {

	client := getOpenAIClient()
	model, err := getModel()
	if err != nil {
		return openai.ChatCompletionResponse{}, fmt.Errorf("Can't get the OpenAI model: %v", err)
	}

	// Get the value of the promptEnvVariable environment variable to use as a system prompt in the API request
	promptTemplate := os.Getenv(promptEnvVariable)
	if promptTemplate == "" {
		return openai.ChatCompletionResponse{}, fmt.Errorf("Prompt not found in the environment variable %s", promptEnvVariable)
	}

	//Add prompt from environment variable as default system prompt
	chatCompletionMessages := []openai.ChatCompletionMessage{{Role: "system", Content: promptTemplate}}

	// Copy chatMessages to ChatCompletionMessages
	for _, v := range chatMessages {
		chatCompletionMessages = append(chatCompletionMessages, openai.ChatCompletionMessage{Role: v.Role, Content: v.Content})
	}

	fmt.Printf("chatCompletionMessages: %v\n", chatCompletionMessages)

	// Send the prompt to OpenAI API and get the response
	response, err := client.CreateChatCompletion(
		context.Background(),

		openai.ChatCompletionRequest{
			Model:    model,
			Messages: chatCompletionMessages,
		},
	)
	if err != nil {
		return openai.ChatCompletionResponse{}, fmt.Errorf("Error sending OpenAI API request: %v", err)
	}

	return response, nil

}

// initOpenAIStream initializes an OpenAI request for stream response and sends it to OpenAI
func initOpenAIStream(promptEnvVariable string, chatMessages []chatMessage) (*openai.ChatCompletionStream, error) {

	client := getOpenAIClient()
	model, err := getModel()
	if err != nil {
		return nil, fmt.Errorf("Can't get the OpenAI model: %v", err)
	}

	// Get the value of the promptEnvVariable environment variable to use as a system prompt in the API request
	promptTemplate := os.Getenv(promptEnvVariable)
	if promptTemplate == "" {
		return nil, fmt.Errorf("Prompt not found in the environment variable %s", promptEnvVariable)
	}

	//Add prompt from environment variable as default system prompt
	chatCompletionMessages := []openai.ChatCompletionMessage{{Role: "system", Content: promptTemplate}}

	// Copy chatMessages to ChatCompletionMessages
	for _, v := range chatMessages {
		chatCompletionMessages = append(chatCompletionMessages, openai.ChatCompletionMessage{Role: v.Role, Content: v.Content})
	}

	fmt.Printf("chatCompletionMessages: %v\n", chatCompletionMessages)

	//PresencePenalty:  2,
	//FrequencyPenalty: 2,

	// Send the prompt to OpenAI API and get the response
	stream, err := client.CreateChatCompletionStream(
		context.Background(),

		openai.ChatCompletionRequest{
			Model:    model,
			Messages: chatCompletionMessages,
			Stream:   true,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("Error sending OpenAI API request: %v", err)
	}

	return stream, nil

}

// getFullOpenAIResponse gets a full response from OpenAI and sends it to the client
func getFullOpenAIResponse(openAIRequest openAIRequest) error {
	response, err := initOpenAIRequest(openAIRequest.request.PromptTemplate, openAIRequest.request.Messages)
	reply := response.Choices[0].Message.Content
	if err != nil {
		return fmt.Errorf("Error sending OpenAI API request: %s", err)
	}
	// Post full answer to websocket
	postInput := &apigatewaymanagementapi.PostToConnectionInput{
		ConnectionId: aws.String(openAIRequest.ConnectionId),
		Data:         []byte(reply),
	}
	_, err = openAIRequest.apiGatewayClient.PostToConnection(postInput)
	if err != nil {
		return fmt.Errorf("Can't post response to websocket: %s\nError: %v", reply, err)
	}

	return fmt.Errorf("Can't get OpenAI API response: %s", reply)
}

// getIntOpenAIResponse gets an integer response from OpenAI, extracts the integer, and sends it to the client
func getIntOpenAIResponse(openAIRequest openAIRequest) error {
	response, err := initOpenAIRequest(openAIRequest.request.PromptTemplate, openAIRequest.request.Messages)
	if err != nil {
		return fmt.Errorf("Error sending OpenAI API request: %v", err)
	}

	// Parse the response and extract integer answer
	reply := response.Choices[0].Message.Content
	fmt.Printf("response.Choices[0].Message.Content: %v\n", response.Choices[0].Message.Content)
	re := regexp.MustCompile(`\[\[(\d+)\]\]`)
	matchInt := re.FindStringSubmatch(reply)
	fmt.Println("matchInt=", matchInt)
	if len(matchInt) > 1 {
		fmt.Println("Number:", matchInt[1])
		postInput := &apigatewaymanagementapi.PostToConnectionInput{
			ConnectionId: aws.String(openAIRequest.ConnectionId),
			Data:         []byte(matchInt[1]),
		}
		_, err = openAIRequest.apiGatewayClient.PostToConnection(postInput)
		if err != nil {
			return fmt.Errorf("Can't post response to websocket: %s\nError: %v", reply, err)
		}
	}

	return fmt.Errorf("Can't parse OpenAI API response: %s", reply)
}

// getStringOpenAIResponse gets a string response from OpenAI, extracts the string, and sends it to the client
func getStringOpenAIResponse(openAIRequest openAIRequest) error {
	response, err := initOpenAIRequest(openAIRequest.request.PromptTemplate, openAIRequest.request.Messages)
	if err != nil {
		return fmt.Errorf("Error sending OpenAI API request: %s", err)
	}

	// Parse the response and extract string answer
	reply := response.Choices[0].Message.Content
	fmt.Printf("response.Choices[0].Message.Content: %v\n", response.Choices[0].Message.Content)
	re := regexp.MustCompile(`\[\[((\w+\s*)+)\]\]`)
	matchString := re.FindStringSubmatch(reply)
	fmt.Println("matchString=", matchString)
	if len(matchString) > 1 {
		fmt.Println("String:", matchString[1])
		postInput := &apigatewaymanagementapi.PostToConnectionInput{
			ConnectionId: aws.String(openAIRequest.ConnectionId),
			Data:         []byte(matchString[1]),
		}
		_, err = openAIRequest.apiGatewayClient.PostToConnection(postInput)
		if err != nil {
			return fmt.Errorf("Can't post response to websocket: %s\nError: %v", reply, err)
		}
	}

	return fmt.Errorf("Can't parse OpenAI API response: %s", reply)
}

// getStreamOpenAIResponse streams responses from OpenAI to the client
func getStreamOpenAIResponse(openAIRequest openAIRequest) error {
	stream, err := initOpenAIStream(openAIRequest.request.PromptTemplate, openAIRequest.request.Messages)
	if err != nil {
		return fmt.Errorf("Error requesting OpenAI API stream: %v", err)
	}

	defer stream.Close()

	postInput := &apigatewaymanagementapi.PostToConnectionInput{
		ConnectionId: aws.String(openAIRequest.ConnectionId),
		Data:         make([]byte, 0),
	}

	for {
		response, err := stream.Recv()
		//isDone := false
		if errors.Is(err, io.EOF) {
			postInput.Data = []byte(endStreamMessage)
			_, err := openAIRequest.apiGatewayClient.PostToConnection(postInput)
			if err != nil {
				return fmt.Errorf("Error requesting OpenAI API stream: %v", err)
			}
			return nil
		}

		if err != nil {
			return fmt.Errorf("Stream error: %v", err)
		}

		postInput.Data = []byte(replaceConfusables(response.Choices[0].Delta.Content))
		_, err = openAIRequest.apiGatewayClient.PostToConnection(postInput)
		if err != nil {
			return fmt.Errorf("Error requesting OpenAI API stream: %v", err)
		}

	}
}
