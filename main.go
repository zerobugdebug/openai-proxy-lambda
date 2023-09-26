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

func getFullOpenAIResponse(promptEnvVariable string, chatMessages []chatMessage) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OpenAI API key not found in environment variable OPENAI_API_KEY")
	}

	client := openai.NewClient(apiKey)

	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = defaultModel
	} else {
		availableModels, err := client.ListModels(context.Background())
		if err != nil {
			fmt.Printf("Error getting list of available models: %s\n Defaulting to %s", err, defaultModel)
			model = defaultModel
		} else {
			if !isValidModel(availableModels.Models, model) {
				fmt.Printf("Model %s is not a valid model\n Defaulting to %s", model, defaultModel)
				model = defaultModel
			}
		}

	}

	promptTemplate := os.Getenv(promptEnvVariable)

	if promptTemplate == "" {
		return "", fmt.Errorf("Prompt not found in the environment variable %s", promptEnvVariable)
	}

	chatCompletionMessages := []openai.ChatCompletionMessage{{Role: "system", Content: promptTemplate}}

	// Copy chatMessages to ChatCompletionMessages
	for _, v := range chatMessages {
		chatCompletionMessages = append(chatCompletionMessages, openai.ChatCompletionMessage{Role: v.Role, Content: v.Content})
		// Fields c and d of arr1[i] will remain their zero values unless set otherwise
	}

	fmt.Printf("chatCompletionMessages: %v\n", chatCompletionMessages)

	// Send the prompt to OpenAI API and get the response

	response, err := client.CreateChatCompletion(
		context.Background(),

		openai.ChatCompletionRequest{
			Model:            "gpt-3.5-turbo",
			MaxTokens:        1000,
			Messages:         chatCompletionMessages,
			PresencePenalty:  2,
			FrequencyPenalty: 2,
		},

		/* 		openai.ChatCompletionRequest{
			Model:     openai.GPT3Dot5Turbo,
			MaxTokens: 1000,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
		}, */
	)
	if err != nil {
		return "", fmt.Errorf("Error sending OpenAI API request: %s", err)
	}

	//fmt.Println("response.Choices[0].Message.Content=", response.Choices[0].Message.Content)
	// Parse the response and extract integer answer
	reply := response.Choices[0].Message.Content
	//fmt.Printf("response.Choices[0].Message.Content: %v\n", response.Choices[0].Message.Content)
	return reply, nil
}

func getIntOpenAIResponse(promptEnvVariable string, chatMessages []chatMessage) (int, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return 0, fmt.Errorf("OpenAI API key not found in environment variable OPENAI_API_KEY")
	}

	client := openai.NewClient(apiKey)

	promptTemplate := os.Getenv(promptEnvVariable)

	if promptTemplate == "" {
		return 0, fmt.Errorf("Prompt not found in the environment variable %s", promptEnvVariable)
	}

	chatCompletionMessages := []openai.ChatCompletionMessage{{Role: "system", Content: promptTemplate}}

	// Copy chatMessages to ChatCompletionMessages
	for _, v := range chatMessages {
		chatCompletionMessages = append(chatCompletionMessages, openai.ChatCompletionMessage{Role: v.Role, Content: v.Content})
		// Fields c and d of arr1[i] will remain their zero values unless set otherwise
	}

	// Send the prompt to OpenAI API and get the response

	response, err := client.CreateChatCompletion(
		context.Background(),

		openai.ChatCompletionRequest{
			Model:            openai.GPT3Dot5Turbo,
			MaxTokens:        1000,
			Messages:         chatCompletionMessages,
			PresencePenalty:  2,
			FrequencyPenalty: 2,
		},
	)
	if err != nil {
		return 0, fmt.Errorf("Error sending OpenAI API request: %s", err)
	}

	//fmt.Println("response.Choices[0].Message.Content=", response.Choices[0].Message.Content)
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
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OpenAI API key not found in environment variable OPENAI_API_KEY")
	}

	client := openai.NewClient(apiKey)

	promptTemplate := os.Getenv(promptEnvVariable)

	if promptTemplate == "" {
		return "", fmt.Errorf("Prompt not found in the environment variable %s", promptEnvVariable)
	}

	chatCompletionMessages := []openai.ChatCompletionMessage{{Role: "system", Content: promptTemplate}}

	// Copy chatMessages to ChatCompletionMessages
	for _, v := range chatMessages {
		chatCompletionMessages = append(chatCompletionMessages, openai.ChatCompletionMessage{Role: v.Role, Content: v.Content})
		// Fields c and d of arr1[i] will remain their zero values unless set otherwise
	}

	// Send the prompt to OpenAI API and get the response

	response, err := client.CreateChatCompletion(
		context.Background(),

		openai.ChatCompletionRequest{
			Model:            openai.GPT3Dot5Turbo,
			MaxTokens:        1000,
			Messages:         chatCompletionMessages,
			PresencePenalty:  2,
			FrequencyPenalty: 2,
		},
	)

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
