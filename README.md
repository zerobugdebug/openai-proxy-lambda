# OpenAI API Websocket Proxy

This project contains a WebSocket proxy for the OpenAI API deployed as an AWS Lambda function written in Go. The proxy facilitates the usage of OpenAI's streaming capabilities, converting the data stream from the OpenAI API into WebSocket messages which are sent to the client. This is particularly useful as the current implementation of AWS Lambda for Go does not support streaming through the lambda function. Additionally, the proxy aids in obscuring OpenAI API authorization information (such as the API key) and system prompts from the client.

## Setup

1. **Clone the repository**
2. **Build the Go application for Linux and ZIP it:**
   ```bash
   GOOS=linux GOARCH=amd64 go build -ldflags="-s -w"
   zip openai-proxy-lambda.zip openai-proxy-lambda
   ```
3. **Use AWS CLI or AWS Lambda console to deploy the `openai-proxy-lambda.zip` to an AWS Lambda function**
4. **Create WebSocket API Gateway:**
   - Navigate to the [AWS Management Console](https://aws.amazon.com/console/).
   - In the "Find Services" search bar, type and select "API Gateway".
   - Click on "Create API".
   - Select the "WebSocket" protocol from the protocol settings.
   - Provide a name for your API in the "Name" field, for example, `OpenAI-WebSocket-Proxy`.
   - Optionally, you can provide a description in the "Description" field.
   - Click on "Create API".
   - Once the API is created, you will be redirected to the API Gateway dashboard for your newly created API.
   - Under the "Routes" section in the left sidebar, click on "Create".
   - Create the following three routes: `$connect`, `$disconnect`, and `$default`.
5. **Configure the following three built-in routes to point to the deployed Lambda function:**
   - `$connect`
   - `$disconnect`
   - `$default`
6. **Environment Variables:**
    - Configure the following 3 environment variables for the AWS Lambda:
        - `OPENAI_API_KEY`: Your OpenAI API key.
        - `OPENAI_MODEL`: The OpenAI model to use (e.g., "gpt-3.5-turbo" or "gpt-4"). If left empty, defaults to "gpt-3.5-turbo".
        - `API_GW_ENDPOINT`: The endpoint of your API Gateway.

## Usage

Make a JSON request to the Websocket URL of AWS API Gateway with the following format:

```json
{
	"prompt_template": "MY_TEMPLATE",
	"messages": [
		{
			"role": "user",
			"content": "Hello"
		}
	],
	"response_type": "full"
}
```

- `prompt_template`: The environment variable name where the system prompt template is stored.
- `messages`: An array of message objects with a `role` (either "user" or "assistant") and `content` (the content of the message).
- `response_type`: Specifies how you want to receive the response. Possible values are:
  - `int`: Parse the output for the first integer value enclosed in double brackets and return that value.
  - `string`: Parse the output for the first string enclosed in double brackets and return that string.
  - `full`: Wait for the full output from the OpenAI API and return everything at once.
  - `stream`: Stream the response from the OpenAI API as received.

The proxy will utilize the value of the `prompt_template` environment variable as a system prompt, append the `messages` as user/assistant prompts, and forward the request to the OpenAI API. The response from the OpenAI API will be handled according to the specified `response_type`, and sent back to the client via WebSocket messages.

## Code Structure

The provided Go code is structured as follows:
- The `main()` function initiates the Lambda function with `lambda.Start(Handler)`.
- The `Handler` function is the entry point for the AWS Lambda function which differentiates between connection, disconnection, and default requests.
- The `handleRequest` function handles the incoming request, parses the request body, and directs the handling to respective functions based on the `response_type`.
- Functions `getIntOpenAIResponse`, `getStringOpenAIResponse`, `getFullOpenAIResponse`, and `getStreamOpenAIResponse` handle the OpenAI API interaction based on the `response_type`.
- Utility functions such as `parseRequestBody`, `errorResponse`, `getAPIGatewayClient`, `createOpenAIRequest`, `isValidModel`, `getOpenAIClient`, and `getModel` facilitate various functionalities required for processing the request and interacting with the OpenAI API.
- Error handling is done throughout the code to ensure that any issues are caught and handled appropriately.

## Notes

- Ensure the OpenAI API key stored in AWS Lambda environment variables is kept confidential.
- You can customize the request to the OpenAI API by modifying the environement variables, constants and parameters in the code.
- Always monitor and manage your OpenAI API and AWS usage to avoid unexpected costs.

## Contributing

Feel free to fork the repository, make changes, and submit pull requests. For major changes, please open an issue first to discuss the proposed change.

---

Enjoy using the OpenAI API Websocket Proxy!
