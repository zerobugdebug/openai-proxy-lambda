package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
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

func Handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var reqBody Request

	fmt.Printf("request.Body: %v\n", request.Body)

	err := json.Unmarshal([]byte(request.Body), &reqBody)
	if err != nil {
		return events.APIGatewayProxyResponse{
			Body:       fmt.Sprintf("Error parsing request JSON: %s", err),
			StatusCode: 400,
		}, nil
	}

	var respBody Response
	switch reqBody.ResponseType {
	case "int":
		{
			openAIResponse, err := getIntOpenAIResponse(reqBody.PromptTemplate, reqBody.Messages)
			if err != nil {
				return events.APIGatewayProxyResponse{
					Body:       fmt.Sprintf("Error calling OpenAI API: %s", err),
					StatusCode: 500,
				}, nil
			}
			respBody.Int = openAIResponse
		}
	case "string":
		{
			openAIResponse, err := getStringOpenAIResponse(reqBody.PromptTemplate, reqBody.Messages)
			if err != nil {
				return events.APIGatewayProxyResponse{
					Body:       fmt.Sprintf("Error calling OpenAI API: %s", err),
					StatusCode: 500,
				}, nil
			}
			respBody.String = openAIResponse
		}
	case "full":
		{
			openAIResponse, err := getFullOpenAIResponse(reqBody.PromptTemplate, reqBody.Messages)
			if err != nil {
				return events.APIGatewayProxyResponse{
					Body:       fmt.Sprintf("Error calling OpenAI API: %s", err),
					StatusCode: 500,
				}, nil
			}
			respBody.Full = openAIResponse
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
	fmt.Printf("responseJSON: %v\n", responseJSON)

	return events.APIGatewayProxyResponse{
		Body:       string(responseJSON),
		StatusCode: 200,
	}, nil
}

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
		return openai.ChatCompletionResponse{}, fmt.Errorf("Error sending OpenAI API request: %s", err)
	}
	return response, nil

}

func getFullOpenAIResponse(promptEnvVariable string, chatMessages []chatMessage) (string, error) {
	response, err := initOpenAIRequest(promptEnvVariable, chatMessages)
	if err != nil {
		return "", fmt.Errorf("Error sending OpenAI API request: %s", err)
	}
	// Parse the response and return full answer
	reply := response.Choices[0].Message.Content
	return reply, nil
}

func getIntOpenAIResponse(promptEnvVariable string, chatMessages []chatMessage) (int, error) {
	response, err := initOpenAIRequest(promptEnvVariable, chatMessages)
	if err != nil {
		return 0, fmt.Errorf("Error sending OpenAI API request: %s", err)
	}

	// Parse the response and extract integer answer
	reply := response.Choices[0].Message.Content
	fmt.Printf("response.Choices[0].Message.Content: %v\n", response.Choices[0].Message.Content)
	re := regexp.MustCompile(`\[\[(\d+)\]\]`)
	matchInt := re.FindStringSubmatch(reply)
	fmt.Println("matchInt=", matchInt)
	if len(matchInt) > 1 {
		fmt.Println("Number:", matchInt[1])
		return strconv.Atoi(matchInt[1])
	}

	return 0, fmt.Errorf("Can't parse OpenAI API response: %s", reply)
}

func getStringOpenAIResponse(promptEnvVariable string, chatMessages []chatMessage) (string, error) {
	response, err := initOpenAIRequest(promptEnvVariable, chatMessages)
	if err != nil {
		return "", fmt.Errorf("Error sending OpenAI API request: %s", err)
	}

	// Parse the response and extract string answer
	reply := response.Choices[0].Message.Content
	fmt.Printf("response.Choices[0].Message.Content: %v\n", response.Choices[0].Message.Content)
	re := regexp.MustCompile(`\[\[((\w+\s*)+)\]\]`)
	matchString := re.FindStringSubmatch(reply)
	fmt.Println("matchString=", matchString)
	if len(matchString) > 1 {
		fmt.Println("String:", matchString[1])
		return matchString[1], nil
	}

	return "", fmt.Errorf("Can't parse OpenAI API response: %s", reply)
}

func main() {
	lambda.Start(Handler)
}
