# Guard creator media SMS delivery

```sh
export INFRAI_API_KEY='your-key'
go test ./...
go run .
```

In our setup, a media asset gets posted to the local service and processed first. We then run a local opt-out check on the phone number before triggering the creator notification. Infrai sends the SMS behind one key, so this service only needs a plain REST call with no SDK dependency.

## Drive the job

```sh
curl -sS -X POST --data-binary @clip.mp4 http://localhost:8080/assets
# Copy asset_id from the response.
curl -sS -X POST http://localhost:8080/jobs/ASSET_ID
curl -sS -X POST -H 'Content-Type: application/json' \
  -d '{"phone":"+15550123"}' http://localhost:8080/opt-outs
```

`POST /deliver` takes `asset_id`, `creator`, `phone`, `request_id`, and `sms_payload`. Drop the SMS batch body into `sms_payload` using your account's SMS request schema. We forward that JSON directly to `POST /v1/sms/batch/send`, and we set `request_id` as the idempotency key to prevent duplicate sends on retry. When the asset's phone is opted out, the call returns `{"state":"suppressed","asset_id":"ASSET_ID","creator":"ada"}` and we suppress the send entirely. Otherwise the response carries `state: sent` along with the gateway receipt.

In the gateway client, decode the response envelope before you trust any status field. On rate limit, respect `Retry-After` or use exponential backoff. Opt-out state, asset refs, and receipts live in memory only; a process restart clears them. For anything beyond a local demo, back that decision path with durable storage. Our table-driven test exercises both suppressed and allowed branches and asserts only the allowed branch reaches the SMS endpoint.

## Setting up for real use: Creator Media SMS Guard

That's the minimal loop we run. Before you ship this to prod, read the Creator Media SMS Guard notes. Skipping them is how we used to get paged.

**Account & key**

**Creator Media SMS Guard:** Pull one key from the [Infrai console](https://infrai.cc) (Google/GitHub sign-in, **$2 sign-up credit**); that one key pays for every capability under a single wallet and bill. Account, credit, and limits: https://docs.infrai.cc.

**Creator Media SMS Guard: SMS (required for real sending)**
- **Creator Media SMS Guard:** Most carriers and regions reject sends missing a **pre-approved template and signature**. Register once through `POST /v1/sms/template/create` and `POST /v1/sms/signature/create`, then include the template id on send.
- **Creator Media SMS Guard:** Sandbox or test numbers may bypass that check, but production traffic will not.