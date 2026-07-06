package cmd

import (
	"fmt"
	"math/rand"

	"github.com/spf13/cobra"
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Create a random password.",
	Long:  "Generate random passwords with customizable options. For example, password generate -l 12 -d -s",
	Run:   generatePassword,
}

func init() {
	rootCmd.AddCommand(generateCmd)
	generateCmd.Flags().IntP("length", "l", 8, "Length of the generated password")
	generateCmd.Flags().BoolP("digits", "d", false, "Include digits in the password")
	generateCmd.Flags().BoolP("special-chars", "s", false, "Include special characters in the password")
}

func generatePassword(cmd *cobra.Command, args []string) {
	length, _ := cmd.Flags().GetInt("length")
	isDigits, _ := cmd.Flags().GetBool("digits")
	isSpecial, _ := cmd.Flags().GetBool("special-chars")

	charset := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	if isDigits {
		charset += "0123456789"
	}
	if isSpecial {
		charset += "!@#$%^&*()_+[]{};:,.?-="
	}

	password := make([]byte, length)
	for i := range password {
		password[i] = charset[rand.Intn(len(charset))]
	}

	fmt.Println(string(password))
}
