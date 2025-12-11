# Create a GitHub App

1. Go to: `https://github.com/organizations/<your-org>/settings/apps`
2. Click **New GitHub App**
3. Deactivate **Webhooks**
4. Add the required permissions: [GitHub App permissions for Renovate](https://docs.renovatebot.com/modules/platform/github/#running-as-a-github-app)
5. Click **Create GitHub App**
6. Generate a private key and save it securely
   - Scroll to the bottom of the App settings page and click **Generate a private key**
7. Note down the **App ID** (shown at the top of the App settings page)
8. Install the GitHub App on the desired repositories
   - Go to the **Install App** section of your GitHub App settings
   - Click **Install** and select the repositories you want to grant access to
9. Note down the **Installation ID**
   - After installing, you are redirected to a URL like:
     `https://github.com/organizations/<your-org>/settings/installations/12345678`
   - The number at the end (`12345678`) is your Installation ID

![Install GitHub App Location.](/docs/assets/github-app-install.png)
