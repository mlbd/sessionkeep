"""Fill a sandbox data directory with realistic synthetic content for looking at the dashboard.
Usage: python -I sample_data.py <sessionkeep.exe> <empty sandbox dir>   (prints the SESSIONKEEP_DATA_DIR to use)"""
import json, os, subprocess, sys

exe, box = sys.argv[1], sys.argv[2]
env = dict(os.environ, SESSIONKEEP_DATA_DIR=os.path.join(box, "data"), SESSIONKEEP_INSTALL_DIR=os.path.join(box, "no-install"))
shop, api = os.path.join(box, "work", "storefront"), os.path.join(box, "work", "billing-api")
for d in (shop, api, os.path.join(box, "work", "clients")):
    os.makedirs(d)

def run(*args, stdin=b""):
    return subprocess.run([exe, *args], input=stdin, capture_output=True, env=env)

def say(event, session, cwd, text):
    body = {"session_id": session, "cwd": cwd, "hook_event_name": event}
    body["last_assistant_message" if event == "Stop" else "prompt"] = text
    run("hook", event, stdin=json.dumps(body).encode())

run("project", "add", shop, "--capture", "conversation", "--name", "Storefront")
run("project", "add", api, "--capture", "extended", "--name", "Billing API")
run("exclude", "add", os.path.join(box, "work", "clients"))
talk = [
    (shop, "s1", "The checkout page shows the wrong total when a discount code and free shipping are both applied. Can you find out why?",
     "The total is computed twice. `applyDiscount` subtracts the code from the subtotal, then `applyShipping` recomputes from the original subtotal and overwrites it. Shipping needs to work from the discounted figure.\n\nI can make `applyShipping` take the running total instead of reading the cart again. That touches `cart/totals.ts` and two tests."),
    (shop, "s1", "Yes, do that. And remember we never round until the very end, only when displaying.",
     "Done. `applyShipping` now takes the running total, and rounding happens only in `formatMoney`. Both tests pass, and I added one for a discount code together with free shipping."),
    (shop, "s2", "Add a banner for the winter sale. It should disappear on its own after the 14th of January.",
     "Added `WinterBanner` with an end date of 14 January, checked on the server so a wrong clock on the visitor's computer does not matter. It renders nothing after that date."),
    (api, "s3", "We decided invoices are kept for seven years and the export job runs at 02:00 UTC. Please note that down.",
     "Noted: invoices kept for seven years, export job at 02:00 UTC."),
    (api, "s3", "The export fails with ECONNRESET: socket hang up at POST /api/v2/invoices/export. What is going on?",
     "The export sends every invoice in one request, and the gateway closes connections after 30 seconds. With this month's volume the request takes about 50 seconds.\n\nSplitting the export into pages of 500 invoices keeps each request under five seconds."),
]
for cwd, session, prompt, reply in talk:
    say("UserPromptSubmit", session, cwd, prompt)
    say("Stop", session, cwd, reply)
run("ingest")
run("remember", "Never round money until the moment it is displayed.", "--scope", "project", "--project", shop, "--yes")
run("remember", "Write commit messages in the imperative mood, under 60 characters.", "--scope", "global", "--yes")
run("remember", "All dates in exports are ISO 8601, in UTC.", "--scope", "project", "--project", api, "--yes")
run("checkpoint", "Checkout total fixed and tested. Winter banner added.\nNext: translate the banner text, then ask Sam to review the totals change.", "--project", shop)
say("UserPromptSubmit", "s4", shop, "Translate the winter banner into German and French.")
print(env["SESSIONKEEP_DATA_DIR"])
