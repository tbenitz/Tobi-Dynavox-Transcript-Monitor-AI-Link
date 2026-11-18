# AAC Link · Transcript Monitor

A portable Windows x64 tray app that turns a local live transcript into touch-friendly response suggestions. It reads text from a separate transcription app. It does not use the microphone, replace the AAC keyboard, or speak on the user's behalf.

## Get started

1. Download **AACLink.exe** from [Releases](https://github.com/tbenitz/Tobi-Dynavox-Transcript-Monitor-AI-Link/releases) and run it on Windows. This is an unsigned portable preview with no installer or administrator requirement.
2. Select the UTF-8 `.txt` file being updated by your existing transcription software, then save. Keep the default **Built-in offline** engine for a first test.
3. Open Notepad and place the cursor in the document. Press **F9**, then tap a suggestion. It should insert text at the caret without pressing Enter. To test layout only, pick **Show overlay (preview)** in the tray.
4. On the AAC device, focus the message bar and press the same hotkey or use an AAC macro that sends it. You can also bind a macro to run `AACLink.exe --toggle` while leaving the message bar in the foreground. Try simulated typing first and clipboard paste second. Set an **AAC window title filter** once you know your window's title.
5. Tap **Dismiss**, press the hotkey again, or let the timeout hide the strip. Right-click the tray icon for pause/resume, settings, reload, and exit. Double-click it to open settings.

The overlay uses `WS_EX_NOACTIVATE`, handles mouse presses without focusing its buttons, and stops if the foreground window or focused control changes. It never invokes the speech button. Normal Windows input rules still apply. If your AAC software does not accept injected text or uses a private message bar, this preview cannot guarantee insertion or read the typed draft. **Test on the specific AAC app and device.** If no draft can be read, Next words gives transcript-context words instead and says so. Clipboard paste replaces your current clipboard. Run both apps without administrator elevation.

## Prediction engines

| Engine | Suggested system | Setup |
| --- | --- | --- |
| Built-in offline | Start here on an I-110 or a low-memory PC | Included. Uses local reply rules, phrase patterns, and a word list. No model download. |
| Qwen2.5 0.5B | 8 GB, fastest optional model | `ollama pull qwen2.5:0.5b-instruct` |
| Qwen2.5 1.5B | 8 GB, try with free RAM | `ollama pull qwen2.5:1.5b-instruct` |
| Qwen2.5 3B | 16 GB, recommended local AI starting point | `ollama pull qwen2.5:3b-instruct` |
| Qwen2.5 7B | 16 GB with headroom, heavier | `ollama pull qwen2.5:7b-instruct` |
| DeepSeek | Cloud, Internet and API key | `deepseek-flash` by default |
| Grok | Cloud, Internet and API key | `grok-4.3` by default |

For Qwen, install [Ollama](https://ollama.com/download/windows), pull the selected model once on a connected PC, and keep the local Ollama service running. AAC Link talks to `127.0.0.1:11434`. Inference can run offline after setup. No model weights or Ollama installer are bundled. RAM tiers are starting points, not performance guarantees. A CPU-only system running transcription concurrently may need a smaller model than its RAM implies. If the selected provider is unavailable or slow, the built-in suggestions remain usable.

Cloud predictions are **off by default**. A cloud selection requires a key and a separate permission checkbox. Only while the real overlay is open, those providers receive the selected recent transcript context and the readable part of the draft before the cursor, if available. The app does not make its own conversation log. Saved keys are encrypted for the current Windows user with DPAPI. Environment variables `DEEPSEEK_API_KEY` and `XAI_API_KEY` are also supported. Model IDs can be overridden.

## Settings

- **Transcript** — file picker, 250/500/1,000 ms polling, and a recent word or sentence window. Use a stable file path such as `live.txt`; if the producer switches to a dated filename, select that file or configure it to also write a stable live file. Pause ignores transcript text written while paused for the current session. It does not pause the separate transcription app.
- **Triggers** — record or type a global shortcut, change the timeout and cooldown, position the prominent red Dismiss button, and optionally restrict insertion to a matching window title.
- **Suggestions & AI** — Quick replies, Next words, or Hybrid with two replies and up to three words. Choose insertion method, optional trailing space or period, and whether to close after use. An incomplete draft word is completed without retyping its existing prefix. If fewer words match, the app shows fewer buttons rather than inserting unrelated completions. Suggestion tiles do not rearrange while the pointer rests over the overlay.
- **Appearance** — top of the current monitor, above a keyboard of configurable height, or custom drag position; lock toggle, button height, text size, spacing, and three themes. No vendor integration is assumed for automatic keyboard placement.

## Build and test

Use Go 1.25.5 on Windows or cross-build on another system. No Go module dependencies are needed.

```powershell
go test -v ./...
go build -o AACLink-smoke.exe .
.\AACLink-smoke.exe --self-test
go build -trimpath -ldflags="-s -w -H=windowsgui" -o AACLink.exe .
```

The automated smoke test checks the non-activating window style, native control creation, and Windows credential encryption. The release is a preview. Passing build or smoke tests is not a TD Snap or Tobii Dynavox hardware certification. See the [manual test guide](docs/manual-test.md). This is an independent utility and is not affiliated with Tobii Dynavox.
