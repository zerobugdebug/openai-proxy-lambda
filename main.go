package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/apigatewaymanagementapi"
	"github.com/sashabaranov/go-openai"
)

const (
	defaultModel = "gpt-3.5-turbo"
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

type Response struct {
	Int    int    `json:"int,omitempty"`
	String string `json:"string,omitempty"`
	Full   string `json:"full,omitempty"`
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

func Handler(ctx context.Context, request events.APIGatewayWebsocketProxyRequest) (events.APIGatewayProxyResponse, error) {

	fmt.Printf("request.Resource: %v\n", request.Resource)
	fmt.Printf("request.Path: %v\n", request.Path)
	fmt.Printf("request.HTTPMethod: %v\n", request.HTTPMethod)
	fmt.Printf("request.Body: %v\n", request.Body)
	fmt.Printf("request.RequestContext: %v\n", request.RequestContext)
	fmt.Printf("request.RequestContext.RouteKey: %v\n", request.RequestContext.RouteKey)

	switch request.RequestContext.RouteKey {
	case "$connect":
		//Nothing special to do
		return events.APIGatewayProxyResponse{StatusCode: 200}, nil
	case "$disconnect":
		//Nothing special to do
		return events.APIGatewayProxyResponse{StatusCode: 200}, nil
	}

	var reqBody Request

	err := json.Unmarshal([]byte(request.Body), &reqBody)
	if err != nil {
		return events.APIGatewayProxyResponse{
			Body:       fmt.Sprintf("Error parsing request JSON: %s", err),
			StatusCode: 400,
		}, nil
	}

	var respBody Response

	// Get the endpoint URL for the gateway from "API_GW_ENDPOINT" environment variable
	apiEndpoint := os.Getenv("API_GW_ENDPOINT")
	if apiEndpoint == "" {
		return events.APIGatewayProxyResponse{
			Body:       fmt.Sprintf("Internal error. Environment configuration is missing."),
			StatusCode: 500,
		}, nil
	}

	openAIRequest := openAIRequest{
		request:          reqBody,
		apiGatewayClient: apigatewaymanagementapi.New(session.Must(session.NewSession()), aws.NewConfig().WithEndpoint(apiEndpoint)),
		ConnectionId:     request.RequestContext.ConnectionID,
	}

	switch reqBody.ResponseType {
	case "int":
		{
			err := getIntOpenAIResponse(openAIRequest)
			if err != nil {
				return events.APIGatewayProxyResponse{
					Body:       fmt.Sprintf("Error calling OpenAI API: %s", err),
					StatusCode: 500,
				}, nil
			}
		}
	case "string":
		{
			err := getStringOpenAIResponse(openAIRequest)
			if err != nil {
				return events.APIGatewayProxyResponse{
					Body:       fmt.Sprintf("Error calling OpenAI API: %s", err),
					StatusCode: 500,
				}, nil
			}
		}
	case "full":
		{
			err := getFullOpenAIResponse(openAIRequest)
			if err != nil {
				return events.APIGatewayProxyResponse{
					Body:       fmt.Sprintf("Error calling OpenAI API: %s", err),
					StatusCode: 500,
				}, nil
			}
		}
	case "stream":
		{
			err := getStreamOpenAIResponse(openAIRequest)
			if err != nil {
				return events.APIGatewayProxyResponse{
					Body:       fmt.Sprintf("Error calling OpenAI API: %s", err),
					StatusCode: 500,
				}, nil
			}
		}
	default:
		return events.APIGatewayProxyResponse{
			Body:       fmt.Sprintf("Incorrect response type: %s", err),
			StatusCode: 500,
		}, nil
	}

	//fmt.Printf("reqBody: %v\n", reqBody)
	//fmt.Printf("respBody: %v\n", respBody)

	responseJSON, err := json.Marshal(respBody)
	if err != nil {
		return events.APIGatewayProxyResponse{
			Body:       fmt.Sprintf("Error converting OpenAI response to JSON: %s", err),
			StatusCode: 500,
		}, nil
	}
	//fmt.Printf("responseJSON: %v\n", responseJSON)

	return events.APIGatewayProxyResponse{
		Body:       string(responseJSON),
		StatusCode: 200,
	}, nil
}

/*
func newWebsocketHandler(apiGatewayEndpoint, apiGatewayStage string) *WebsocketHandler {
	sess := session.Must(session.NewSession())
	apiGatewayClient := apigatewaymanagementapi.New(sess, aws.NewConfig().WithEndpoint(apiGatewayEndpoint))
	return &WebsocketHandler{apiGatewayClient: apiGatewayClient, apiGatewayStage: apiGatewayStage}
}

// Send a message to a specific client.
func (ws *WebsocketHandler) sendMessage(connectionID, message string) error {
	postInput := &apigatewaymanagementapi.PostToConnectionInput{
		ConnectionId: aws.String(connectionID),
		Data:         []byte(message),
	}

	_, err := ws.apiGatewayClient.PostToConnection(postInput)
	if err != nil {
		return err
	}

	return nil
} */

func isValidModel(models []openai.Model, id string) bool {
	for _, model := range models {
		if model.ID == id {
			return true
		}
	}
	return false
}

func initOpenAIRequest(promptEnvVariable string, chatMessages []chatMessage) (openai.ChatCompletionResponse, error) {

	// Get the value of the "OPENAI_API_KEY" environment variable
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return openai.ChatCompletionResponse{}, fmt.Errorf("OpenAI API key not found in environment variable OPENAI_API_KEY")
	}

	client := openai.NewClient(apiKey)

	// Get the value of the "OPENAI_MODEL" environment variable
	model := os.Getenv("OPENAI_MODEL")
	// Check if the model value is empty
	if model == "" {
		// If the model value is empty, set it to the default model
		model = defaultModel
	} else {
		// Otherwise, retrieve a list of available models
		availableModels, err := client.ListModels(context.Background())
		if err != nil {
			// Print an error message and set the model to the default model
			fmt.Printf("Error getting list of available models: %s\n Defaulting to %s", err, defaultModel)
			model = defaultModel
		} else {
			// Check if the provided model is valid
			if !isValidModel(availableModels.Models, model) {
				// If it's not a valid model, print a message and set the model to the default model
				fmt.Printf("Model %s is not a valid model\n Defaulting to %s", model, defaultModel)
				model = defaultModel
			}
		}
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
			Model:            model,
			Messages:         chatCompletionMessages,
			PresencePenalty:  2,
			FrequencyPenalty: 2,
		},
	)
	if err != nil {
		return openai.ChatCompletionResponse{}, fmt.Errorf("Error sending OpenAI API request: %v", err)
	}

	return response, nil

}

func initOpenAIStream(promptEnvVariable string, chatMessages []chatMessage) (*openai.ChatCompletionStream, error) {

	// Get the value of the "OPENAI_API_KEY" environment variable
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key not found in environment variable OPENAI_API_KEY")
	}

	client := openai.NewClient(apiKey)

	// Get the value of the "OPENAI_MODEL" environment variable
	model := os.Getenv("OPENAI_MODEL")
	// Check if the model value is empty
	if model == "" {
		// If the model value is empty, set it to the default model
		model = defaultModel
	} else {
		// Otherwise, retrieve a list of available models
		availableModels, err := client.ListModels(context.Background())
		if err != nil {
			// Print an error message and set the model to the default model
			fmt.Printf("Error getting list of available models: %s\n Defaulting to %s", err, defaultModel)
			model = defaultModel
		} else {
			// Check if the provided model is valid
			if !isValidModel(availableModels.Models, model) {
				// If it's not a valid model, print a message and set the model to the default model
				fmt.Printf("Model %s is not a valid model\n Defaulting to %s", model, defaultModel)
				model = defaultModel
			}
		}
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

	// Send the prompt to OpenAI API and get the response
	stream, err := client.CreateChatCompletionStream(
		context.Background(),

		openai.ChatCompletionRequest{
			Model:            model,
			Messages:         chatCompletionMessages,
			PresencePenalty:  2,
			FrequencyPenalty: 2,
			Stream:           true,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("Error sending OpenAI API request: %v", err)
	}

	return stream, nil

}

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

func getStreamOpenAIResponse(openAIRequest openAIRequest) error {
	stream, err := initOpenAIStream(openAIRequest.request.PromptTemplate, openAIRequest.request.Messages)
	if err != nil {
		return fmt.Errorf("Error reqeusting OpenAI API stream: %v", err)
	}

	defer stream.Close()

	postInput := &apigatewaymanagementapi.PostToConnectionInput{
		ConnectionId: aws.String(openAIRequest.ConnectionId),
		Data:         make([]byte, 0),
	}

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			fmt.Println("Stream finished")
			return nil
		}

		if err != nil {
			return fmt.Errorf("Stream error: %v", err)
		}

		postInput.Data = []byte(response.Choices[0].Delta.Content)
		_, err = openAIRequest.apiGatewayClient.PostToConnection(postInput)
		if err != nil {
			return fmt.Errorf("Error reqeusting OpenAI API stream: %v", err)
		}

	}
}

func main() {
	lambda.Start(Handler)
}
