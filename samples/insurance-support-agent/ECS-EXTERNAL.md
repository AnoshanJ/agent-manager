# Run the insurance agent externally on Amazon ECS

## Register in Agent Manager SaaS

1. Sign in to the Agent Manager console and select your organization and project.
2. Choose **Add Agent → Externally-Hosted Agent**, name the agent, and choose **Register**.
3. In **Setup Agent**, select a token duration and choose **Generate**. Store the key
   securely as `AMP_AGENT_API_KEY` and copy the supplied instrumentation endpoint
   as `AMP_OTEL_ENDPOINT`.

## Build and run

Build from the `agent` directory:

```sh
docker build --platform linux/amd64 -f Dockerfile.ecs -t insurance-agent:latest .
```

Push the image to your Amazon ECR repository using its push instructions. In the
ECS console, create a task definition with the image, port 8000, a log destination,
and a task execution role permitted to pull the image and read the required secrets.

The container starts with `amp-instrument python main.py`. Configure
`AMP_OTEL_ENDPOINT` as an environment variable and inject `AMP_AGENT_API_KEY`
from AWS Secrets Manager. For the default model, also inject `OPENAI_API_KEY`;
`OPENAI_MODEL` defaults to `gpt-4o-mini`. For Amazon Bedrock, follow
[BEDROCK-GATEWAY.md](BEDROCK-GATEWAY.md) instead of supplying an OpenAI key.
Never bake credentials into the image.

In your ECS cluster, run one Fargate task with the prepared task definition,
subnet, and security group. Allow outbound HTTPS to the model and instrumentation
endpoints. Restrict port 8000 to your test client. Use Fargate Linux 1.4.0 or later
when injecting individual Secrets Manager JSON fields.

Check `GET /health`, send a synthetic request to `POST /chat`, and open the agent's
**Traces** page in Agent Manager. Verify that model and tool spans appear. Check
instrumentation startup and export errors in the container logs if traces are missing.

Telemetry export is separate from model routing and HTTP endpoint security.
Registration does not protect the agent endpoint. Session IDs are caller-supplied,
not authenticated identities; the sample keeps data and session state in memory.
Use TLS and authentication before allowing production callers.

## Clean up

Stop the task (and remove or scale down any service that would replace it).
Revoke dedicated credentials and delete test-only images, logs, secrets, and
agent registrations. Preserve shared VPCs and other shared resources.
