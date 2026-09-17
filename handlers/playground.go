package handlers

// playgroundHTML is the GraphQL Playground UI.
//
// It's a self-contained HTML page that loads Playground from a CDN.
// The page auto-detects WebSocket support for subscriptions.
const playgroundHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>GraphQL Playground</title>
  <style>
    body { margin: 0; padding: 0; font-family: sans-serif; }
    #root { height: 100vh; }
  </style>
</head>
<body>
  <div id="root"></div>
  <script src="https://cdn.jsdelivr.net/npm/graphql-playground-react@1.7.20/build/static/js/middleware.js"></script>
  <script>
    window.addEventListener('load', function() {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = protocol + '//' + window.location.host + '/subscriptions';

      GraphQLPlayground.init(document.getElementById('root'), {
        endpoint: window.location.pathname,
        subscriptionEndpoint: wsUrl,
        settings: {
          'request.credentials': 'include',
          'editor.theme': 'dark'
        }
      });
    });
  </script>
</body>
</html>`
