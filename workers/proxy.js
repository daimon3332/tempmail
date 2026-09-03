// Reverse proxy: keeps tempmail.daimon.dpdns.org on Cloudflare while the
// backend runs on the self-hosted origin. Adds the origin token so the
// backend rejects direct-IP traffic.
export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    const target = new URL(env.ORIGIN_URL);
    url.protocol = target.protocol;
    url.host = target.host;
    const headers = new Headers(request.headers);
    headers.set("x-origin-token", env.ORIGIN_TOKEN);
    headers.set("x-forwarded-host", request.headers.get("host") || "");
    headers.set("x-forwarded-proto", "https");
    const init = { method: request.method, headers, redirect: "manual" };
    if (!["GET", "HEAD"].includes(request.method)) init.body = request.body;
    return fetch(url.toString(), init);
  },
};
