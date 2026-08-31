# Third-party licenses

The personal profile and TOTP flow uses the following directly imported open-source packages:

| Package | Version | License | Purpose |
| --- | --- | --- | --- |
| `github.com/pquerna/otp` | `v1.5.0` | Apache License 2.0 | TOTP secret generation, `otpauth://` key construction, code generation, and verification |
| `qrcode.react` | `4.2.0` | ISC | Rendering the `otpauth://` enrollment QR code in the console |
| `github.com/openai/openai-go/v2` | `v2.7.1` | Apache License 2.0 | Official OpenAI SDK for channel model discovery and chat completions |
| `github.com/anthropics/anthropic-sdk-go` | `v1.68.0` | MIT License | Official Anthropic SDK for model discovery and Messages API calls |
| `google.golang.org/genai` | `v1.58.0` | Apache License 2.0 and BSD-3-Clause components | Official Gemini SDK for model discovery and GenerateContent calls |

Both licenses permit commercial use subject to retaining the applicable copyright and license notices in redistributed source or notices.
