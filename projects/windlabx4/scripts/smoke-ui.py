"""WindLabX4 UI Smoke Test

Launches Go API + Vite dev server, opens the app in Playwright,
verifies key UI elements render correctly, and captures a screenshot.

Usage:
    python scripts/smoke-ui.py

Requires: playwright (pip install playwright && playwright install chromium)
"""

import sys
from pathlib import Path

REPO = Path(__file__).resolve().parents[1]
FRONTEND = REPO / "apps/desktop-wails/frontend"
API_DIR = REPO / "services/api-go"
SCREENSHOT_DIR = REPO / "build"
SCREENSHOT_DIR.mkdir(parents=True, exist_ok=True)

SKILL_HELPER = r"C:\Users\wuzhy\.opencode\skills\webapp-testing\scripts\with_server.py"


def main():
    import subprocess

    servers = [
        ("--server", f"go run ./cmd/server/main.go", "--port", "8080"),
        ("--server", f"npm --prefix {FRONTEND} run dev -- --host 127.0.0.1", "--port", "5173"),
    ]

    # Build the helper command
    cmd = [sys.executable, str(SKILL_HELPER)]
    for srv_args in servers:
        cmd.extend(srv_args)
    cmd.extend(["--timeout", "60", "--", sys.executable, "-c", smoke_script(SCREENSHOT_DIR)])

    result = subprocess.run(
        cmd,
        cwd=str(API_DIR),
        capture_output=True,
        text=True,
        timeout=180,
    )
    print(result.stdout)
    if result.stderr:
        print(result.stderr, file=sys.stderr)

    if result.returncode != 0:
        print("SMOKE TEST FAILED", file=sys.stderr)
        sys.exit(1)


def smoke_script(screenshot_dir: Path) -> str:
    s = str(screenshot_dir / "WindLabX4-smoke.png").replace("\\", "\\\\")
    return f"""
from playwright.sync_api import sync_playwright
out = r"{s}"
with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    page = browser.new_page(viewport={{'width': 1440, 'height': 900}})
    errors = []
    page.on('console', lambda msg: errors.append(f'{{msg.type}}:{{msg.text}}') if msg.type == 'error' else None)
    page.goto('http://127.0.0.1:5173', wait_until='networkidle')
    page.screenshot(path=out, full_page=True)
    title = page.locator('h1').inner_text(timeout=5000).strip()
    rail = page.locator('nav button, [class*="rail"] button, [class*="app-rail"] button').count()
    screenshots = page.locator('div[class*="channel-card"], article[class*="channel-card"]').count()
    print(f'title={{title}}')
    print(f'rail_buttons={{rail}}')
    print(f'channel_cards={{screenshots}}')
    print(f'screenshot={{out}}')
    if errors:
        print('CONSOLE_ERRORS=' + '|'.join(errors))
    assert 'Wind' in title, f'Expected title to contain Wind, got {{title}}'
    assert rail >= 5, f'Expected at least 5 rail buttons, got {{rail}}'
    assert screenshots >= 1, f'Expected channel cards, got {{screenshots}}'
    print('SMOKE TEST PASSED')
    browser.close()
"""


if __name__ == "__main__":
    main()
