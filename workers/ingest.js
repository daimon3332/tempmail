// Email Routing target: forwards every inbound message to the self-hosted
// backend. If the backend is unreachable the message is rejected with a
// temporary failure so the sender retries.
export default {
  async email(message, env) {
    const raw = new Uint8Array(await new Response(message.raw).arrayBuffer());
    let bin = "";
    for (let i = 0; i < raw.length; i += 0x8000) {
      bin += String.fromCharCode.apply(null, raw.subarray(i, i + 0x8000));
    }
    const res = await fetch(env.ORIGIN_URL + "/external/ingest", {
      method: "POST",
      headers: {
        "content-type": "application/json",
        "x-ingest-token": env.INGEST_TOKEN,
        "x-origin-token": env.ORIGIN_TOKEN,
      },
      body: JSON.stringify({ from: message.from, to: message.to, raw_base64: btoa(bin) }),
    });
    if (!res.ok) {
      const body = await res.text();
      if (res.status >= 500 || res.status === 401 || res.status === 403) {
        throw new Error(`backend ${res.status}: ${body}`);
      }
      message.setReject(body.slice(0, 200) || "rejected");
    }
  },
};
