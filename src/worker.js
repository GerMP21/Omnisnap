// Serves the built schema site and keeps omnisnap.org canonical.
//
// The static assets come from _site (tools/site/build.py maps each schema's
// $id to its published path). run_worker_first is on, so every request reaches
// this Worker: it redirects www to the apex and adds the headers the schema
// responses need — `_headers` rules do not apply to Worker responses.
export default {
  async fetch(request, env) {
    const url = new URL(request.url);
    if (url.hostname === "www.omnisnap.org") {
      url.hostname = "omnisnap.org";
      url.protocol = "https:";
      return Response.redirect(url.toString(), 301);
    }
    const res = await env.ASSETS.fetch(request);
    if (!url.pathname.startsWith("/schema/")) {
      return res;
    }
    const headers = new Headers(res.headers);
    headers.set("Access-Control-Allow-Origin", "*");
    headers.set("X-Content-Type-Options", "nosniff");
    return new Response(res.body, { status: res.status, headers });
  },
};
