# AAC Link v0.1

Windows x64 native desktop utility. Keep transcription, prediction, and input separate. Use a focus-preserving overlay with standard accessible buttons, a system tray menu, and four settings tabs. Capture and revalidate the active top-level window and focused child before inserting anything. Never send Enter or invoke Speak. Show the captured app name and allow a foreground title filter. Read a focused Windows Edit control when exposed. Otherwise only offer transcript-context words and disclose that no AAC draft is available.

Build order and checks:

1. Test bounded transcript context, privacy pause/resume, exact mode counts, prefix completion, and selection planning. Implement the deterministic offline engine and a transcript reader.
2. Test model output and provider payloads. Implement optional localhost Ollama and opt-in DeepSeek and Grok. Render offline suggestions immediately and cancel obsolete requests.
3. Build the Win32 UI, settings, hotkey, focus and input handling. Run scripted native tests on Windows and cross-build the portable EXE. Keep live suggestions stable while the pointer is over the strip.
4. Publish a preview and document the manual test for Notepad and AAC devices. Hardware and AAC compatibility are separate from build verification.
