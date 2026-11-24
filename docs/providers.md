# AI providers and API keys

Besides the command-line AI clients (Claude Code, Codex, Antigravity), the app can ask an **AI
provider** directly, using a key you save. Pick whichever suits you. You can keep both and let the app
fall back from one to the other.

## What is an API key?

An API key is a long private password that a provider gives your account. The app sends it with each
question so the provider knows whom to bill.

- **It bills your own account.** Every question costs a little (a typical answer is a few cents or less
  on a small model). Set a spending limit in the provider's console if you want a ceiling.
- **Keep it private.** The app stores it in Windows Credential Manager (the entry is named
  `wowc-advisor:<provider>`). On macOS and Linux it is a file in the app's data folder that only your
  user can read. It is never written to a settings file or a log, and the app never shows it again after
  you save it. You can delete it any time.

## Free options

- **Ollama** runs an open model on your own PC: free, private, and no key. Install it from
  [ollama.com](https://ollama.com), pull a model that supports tools, and keep it running.
- **LM Studio** is the same idea with a point-and-click app.
- **OpenRouter** has some free models. Pick one from the model list after you save a key.

Small local models can be slower and less accurate than the big hosted ones. They must support
**tools**; if a model doesn't, the app says "This model can't use tools; pick another."

## Subscriptions are different

A Claude.ai or ChatGPT **subscription** is not an API key, and those providers allow only API keys in
other apps. To use a subscription, use the command-line path (Claude Code or Codex) from
[Getting started](getting-started.md#set-up-your-ai-client).

## The providers

| Provider | Address | Key | Models offered (efficient / balanced / best) |
|---|---|---|---|
| Anthropic (Claude) | `https://api.anthropic.com/v1` | yes | `claude-haiku-5-5` / `claude-sonnet-5-5` / `claude-opus-5-5` |
| OpenAI | `https://api.openai.com/v1` | yes | `gpt-6-luna` / `gpt-6.2-sol` / `gpt-6.2-astra` |
| OpenRouter | `https://openrouter.ai/api/v1` | yes | `anthropic/claude-haiku-5.5` / `anthropic/claude-sonnet-5.5` / `anthropic/claude-opus-5.5` |
| Ollama | `http://localhost:11434/v1` | no | the models you installed |
| LM Studio | `http://localhost:1234/v1` | no | the models you loaded |
| Other | your server's address | optional | any model id you type |

Model names change; choose **custom** to type a newer one, or pick from the list the provider returns.

## Set up

In the app's settings, open **AI**, choose a provider, paste the key, pick a model (or leave the tier on
efficient), and press **Test connection**. The test sends one tiny message with no game data.

## For the curious: the local API

The app's local server has these endpoints (all need the app's token): `GET /api/providers`,
`POST /api/providers/active`, `POST /api/providers/clients` (the ordered list, tried in order when one
can't start), `POST /api/providers/key` and `POST /api/providers/key/delete` (write-only),
`GET /api/providers/models`, and `POST /api/providers/test`. The saved choices live in
`providers.json` in the app's data folder and hold no keys. With nothing saved there, the app behaves
as it always did.
