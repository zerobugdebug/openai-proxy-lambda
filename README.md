# OpenAI API Proxy for AWS Lambda

This project provides a proxy function designed to be deployed in AWS Lambda. It allows clients to securely access the OpenAI API without exposing the OpenAI API key. The API key and request parameters are stored as environment variables within AWS Lambda. Proxy supports only ChatCompletion request with model defined in the environement variables.

## Overview

The provided Go code contains an AWS Lambda function which:

1. Reads the request parameters (including the desired response type, prompt template, and chat messages).
2. Depending on the specified response type (`int`, `string`, or `full`), the function calls the OpenAI API and fetches the response.
3. Parses the OpenAI API response and sends it back to the client.

## Setup

### Prerequisites

- AWS CLI
- AWS Lambda
- Go environment setup
- OpenAI account

## Instructions

### Deployment Steps
1. Clone the repository.
2. Build the Go application for Linux and ZIP it.
   ```
   GOOS=linux GOARCH=amd64 go build -ldflags="-s -w"
   zip openai-proxy-lambda.zip openai-proxy-lambda
   ```
3. Use AWS CLI or AWS Lambda console to deploy the `deployment.zip` to an AWS Lambda function.
4. Set the following environment variables in AWS Lambda:
    - `OPENAI_API_KEY`: Your OpenAI API key.
    - `OPENAI_MODEL`: (optional) OpenAI model you wish to use. Defaults to `gpt-3.5-turbo`.
    - `YOUR_ENV_VARIABLE_FOR_PROMPT_TEMPLATE`: System prompt to use for the request to OpenAI API
5. Use the Lambda function

    To access the OpenAI API through the proxy, send a POST request to the Lambda API Gateway endpoint with a body like:

    ```json
    {
      "prompt_template": "YOUR_ENV_VARIABLE_FOR_PROMPT_TEMPLATE",
      "messages": [
        {"role": "user", "content": "Your question here"}
      ],
      "response_type": "int" // or "string" or "full"
    }
    ```

    The response will contain the result from OpenAI in the specified format.


### Regex Parsing

In the code, regular expressions (regex) are utilized to extract specific types of data from the OpenAI API response. Below is the breakdown:

#### String Parsing

Pattern:
```
\[\[((\w+\s*)+)\]\]
```

Explanation:
- `\[\[`: Matches the starting `[[`.
- `(\w+\s*)+`: Matches words and spaces. This is to capture multiple words with spaces in between.
- `\]\]`: Matches the ending `]]`.

#### Int Parsing

Pattern:
```
\[\[(\d+)\]\]
```

Explanation:
- `\[\[`: Matches the starting `[[`.
- `(\d+)`: Matches a series of digits, capturing integer values.
- `\]\]`: Matches the ending `]]`.

These regex patterns are designed to extract data enclosed within `[[ ]]` from the OpenAI API response.


## Notes

- Ensure the OpenAI API key stored in AWS Lambda environment variables is kept confidential.
- You can customize the request to the OpenAI API by modifying the constants and parameters in the code.
- Always monitor and manage your OpenAI API usage to avoid unexpected costs.

## Contributing

Feel free to fork the repository, make changes, and submit pull requests. For major changes, please open an issue first to discuss the proposed change.

---

Enjoy using the OpenAI Proxy for AWS Lambda!
