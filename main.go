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

type Request struct {
	PromptTemplate string `json:"prompt_template"`
	PromptData     string `json:"prompt_data"`
}

func Handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var reqBody Request

	err := json.Unmarshal([]byte(request.Body), &reqBody)
	if err != nil {
		return events.APIGatewayProxyResponse{
			Body:       fmt.Sprintf("Error parsing request JSON: %s", err),
			StatusCode: 400,
		}, nil
	}

	openAIResponse, err := getOpenAIResponse(reqBody.PromptTemplate, reqBody.PromptData)
	if err != nil {
		return events.APIGatewayProxyResponse{
			Body:       fmt.Sprintf("Error calling OpenAI API: %s", err),
			StatusCode: 500,
		}, nil
	}
	fmt.Printf("openAIResponse: %v\n", openAIResponse)

	responseJSON, err := json.Marshal(openAIResponse)
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

func getOpenAIResponse(promptEnvVariable string, promptData string) (interface{}, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return 0, fmt.Errorf("OpenAI API key not found in environment variable OPENAI_API_KEY")
	}

	client := openai.NewClient(apiKey)

	promptTemplate := os.Getenv(promptEnvVariable)

	if promptTemplate == "" {
		return 0, fmt.Errorf("Prompt not found in the environment variable %s", promptEnvVariable)
	}

	// Send the prompt to OpenAI API and get the response
	prompt := fmt.Sprintf(promptTemplate, promptData)

	response, err := client.CreateChatCompletion(
		context.Background(),

		openai.ChatCompletionRequest{
			Model:     openai.GPT3Dot5Turbo,
			MaxTokens: 1000,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
		},
	)
	if err != nil {
		return 0, fmt.Errorf("Error sending OpenAI API request: %s", err)
	}

	//fmt.Println("response.Choices[0].Message.Content=", response.Choices[0].Message.Content)
	// Parse the response and extract integer answer
	reply := response.Choices[0].Message.Content
	re := regexp.MustCompile(`\[\[(\d+)\]\]`)
	matchInt := re.FindStringSubmatch(reply)
	fmt.Println("matchInt=", matchInt)
	if len(matchInt) > 1 {
		fmt.Println("Number:", matchInt[1])
		return strconv.Atoi(matchInt[1])
	}
	re = regexp.MustCompile(`\[\[(\w+)\]\]`)
	matchString := re.FindStringSubmatch(reply)
	fmt.Println("matchString=", matchString)
	if len(matchString) > 1 {
		fmt.Println("String:", matchString[1])
		return matchString[1], nil
	}

	return 0, fmt.Errorf("Can't parse OpenAI API response: %s", reply)
}

func main() {
	lambda.Start(Handler)
}
