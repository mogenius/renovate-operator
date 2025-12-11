# Create a new GitHub App
1. Go to: https://github.com/organizations/mogenius/settings/apps 
2. Click on "New GitHub App"
3. Desactivate Webhooks
4. Add required Permissions
  - Contents: Read & Write
  - Issues: Read & Write
  - Pull Requests: Read & Write
  - Repository metadata: Read-only
5. Click on "Create GitHub App"
6. Generate a private key and save it securely
  - Hint: At the bottom of the page
7. Note down the App ID and the Certificate
8. Install the GitHub App on the desired repositories
  - Go to the "Install App" section of your GitHub App settings
  - Click on "Install" and select the repositories you want to grant access to
