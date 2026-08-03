addEventListener("fetch", (e) =>
  e.respondWith(
    new Response("no apps deployed yet", {
      status: 503,
      headers: { "content-type": "text/plain" },
    })
  )
);
