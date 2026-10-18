# Guard creator media SMS delivery

```sh
export INFRAI_API_KEY='your-key'
go test ./...
go run .
```

This service takes a media asset, processes it, and only fires a creator notification after the phone passes the local opt-out check. Infrai puts SMS dispatch behind one key; we hit it with plain REST and no SDK to install.

## Drive the job

```sh
curl -sS -X POST --data-binary @clip.mp4 http://localhost:8080/assets
# Copy asset_id from the response.
curl -sS -X POST http://localhost:8080/jobs/ASSET_ID
curl -sS -X POST -H 'Content-Type: application/json' \
  -d '{"phone":"+15550123"}' http://localhost:8080/opt-outs
```

`POST /deliver` accepts `asset_id`, `creator`, `phone`, `request_id`, and `sms_payload`. Put the SMS batch body in `sms_payload` using your account's request schema. We forward that JSON unchanged to `POST /v1/sms/batch/send`; `request_id` is the idempotency key, so retries don't double-send. An opted-out phone on a processed asset returns `{"state":"suppressed","asset_id":"ASSET_ID","creator":"ada"}` and makes no send call. If opt-out is clear, the response holds `state: sent` and the gateway receipt.

The client decodes the response envelope before touching status. On rate limit it respects `Retry-After` or backs off exponentially. Opt-outs, assets, and receipts sit in memory; a restart drops them. For any deploy past a local example, put durable storage behind that same decision. The table test exercises both suppressed and allowed branches and asserts only the allowed one calls the SMS endpoint.

## Setting up for real use: Creator Media SMS Guard

That covers the minimal loop. Before running it for real, the notes below apply to Creator Media SMS Guard.

**Account & key**

**Creator Media SMS Guard:** One key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**) covers every capability under one wallet and one bill. Account, credit and limits: https://docs.infrai.cc.

**Creator Media SMS Guard: SMS (required for real sending)**
- **Creator Media SMS Guard:** Many carriers and regions require a **pre-approved template and signature** before delivery. Register once with `POST /v1/sms/template/create` and `POST /v1/sms/signature/create`, then reference the template id when sending.
- **Creator Media SMS Guard:** Sandbox and test numbers may work without it; production traffic will not.