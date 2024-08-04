package gitgen

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/seymahandekli/git-gen/pkg/models"
)

//go:generate stringer -type=PromptType
type PromptType int

const (
	PromptCommitMessage PromptType = iota
	PromptCodeReview
	PromptTestCase
)

var (
	ErrUnknownPlatform = errors.New("unknown platform")
)

func runDiffOnCli(config Config) (string, error) {
	// Define the Git command
	cmd := exec.Command("git", "diff", config.SourceRef, config.DestinationRef)
	if config.DestinationRef == "" {
		cmd = exec.Command("git", "diff", config.SourceRef)
	}

	// Create buffers to capture the output and error
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	// Convert the output to a string
	return stdout.String(), nil
}

func runDiffWithGoGit(config Config) (string, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	repo, err := git.PlainOpenWithOptions(workingDir, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return "", err
	}

	srcRefName := plumbing.ReferenceName(config.SourceRef)
	if err := srcRefName.Validate(); err != nil {
		return "", err
	}
	srcRef, err := repo.Reference(srcRefName, true)
	if err != nil {
		return "", err
	}
	srcCommit, err := repo.CommitObject(srcRef.Hash())
	if err != nil {
		return "", err
	}
	srcTree, err := srcCommit.Tree()
	if err != nil {
		return "", err
	}

	var destRef *plumbing.Reference

	if config.DestinationRef != "" {
		destRefName := plumbing.ReferenceName(config.DestinationRef)
		if err := destRefName.Validate(); err != nil {
			return "", err
		}
		destRef, err = repo.Reference(destRefName, true)
		if err != nil {
			return "", err
		}
	} else {
		destRef, err = repo.Storer.Reference(plumbing.HEAD)
		if err != nil {
			return "", err
		}
	}

	destCommit, err := repo.CommitObject(destRef.Hash())
	if err != nil {
		return "", err
	}
	destTree, err := destCommit.Tree()
	if err != nil {
		return "", err
	}

	patch, err := destTree.Diff(srcTree)
	if err != nil {
		return "", err
	}

	return patch.String(), nil
}

func getPrompt(promptType PromptType) string {
	var prompt string
	switch promptType {
	case PromptCommitMessage:
		prompt = "please generate a git commit message with a simple explanation from the changes stated above which is an output of a git diff command. all response of this message should be wrapped in a markdown format because it will be shared in a text-only terminal interface."

	case PromptCodeReview:
		prompt = "please perform a efficient and concise code review which points out crucial improvements could be changed on the target code. the target code is stated above which is an output of a git diff command. all response of this message should be wrapped in a markdown format because it will be shared in a text-only terminal interface."

	case PromptTestCase:
		prompt = "Please generate detailed test cases from the changes stated above, which is an output of a git diff command. The test cases should be comprehensive and cover all the modifications, additions, and deletions in the code. All responses to this message should be wrapped in a markdown format because it will be shared in a text-only terminal interface. Ensure that the test cases include the following details\n- Description,\n- Steps, Detailed steps to execute the test case. \n- Expected Result, The expected outcome of the test case.\n- Actual Result, (This will be filled out during testing.)"
	}

	return prompt
}

func Do(promptType PromptType, config Config) (string, error) {
	// Run the git diff command
	userPrompt, err := runDiffOnCli(config)
	if err != nil {
		return "", err
	}

	systemPrompt := getPrompt(promptType)

	fmt.Println("System Prompt:")
	fmt.Println(systemPrompt)
	fmt.Println("User Prompt:")
	fmt.Println(userPrompt)

	modelConfig := models.ModelConfig{
		PlatformApiKey:              config.PlatformApiKey,
		Platform:                    config.Platform,
		Model:                       config.Model,
		PromptMaxTokens:             config.PromptMaxTokens,
		PromptRequestTimeoutSeconds: config.PromptRequestTimeoutSeconds,
	}

	var runtime models.Model

	switch modelConfig.Platform {
	case "openai":
		runtime = models.NewOpenAi(modelConfig)
	case "ollama":
		runtime, err = models.NewOllamaAi(modelConfig)

		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unknown platform %s - %w", modelConfig.Platform, ErrUnknownPlatform)
	}

	response, err := runtime.ExecPrompt(context.Background(), systemPrompt, userPrompt)
	if err != nil {
		return "", err
	}

	fmt.Println("Model Response:")
	return response.Content, nil
}
