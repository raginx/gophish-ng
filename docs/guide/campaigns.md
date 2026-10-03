---
description: "How to configure and launch phishing campaigns, and monitor opens, clicks, and submissions in Gophish-NG."
---

# Campaigns

Gophish is centered around launching campaigns. This involves sending emails to one or more groups and monitoring for opened emails, clicked links, or submitted credentials.

## Launching a Campaign

To configure and launch a campaign, click the "Campaigns" entry in the navigation sidebar.

![New campaign dialog](../assets/screen-shot-2018-10-08-at-10.43.52-pm.png)

Setting up a campaign requires the following fields to be provided:

* **Name** - The name of the campaign
* **Email Template** - The email that is sent to campaign recipients. This is created in the [Email Templates](templates.md) section of the documentation.
* **Landing Page** - The HTML that is returned when a recipient clicks the link in the email template. This is created in the [Landing Pages](landing-pages.md) section of the documentation.
* **URL** - This is the URL that populates the `{{.URL}}` template value, commonly used in email templates. This should be a URL or IP address that points to the Gophish phishing server and is reachable by the recipient.
* **Launch Date** - This is the date that the campaign will begin. See Scheduling Campaigns for more information.
* **Send Emails By** - This is the date all emails will be sent by. See Scheduling Campaigns for more information.
* **Sending Profile** - This is the SMTP configuration to use when sending emails. This is created in the [Sending Profiles](sending-profiles.md) section of the documentation.
* **Groups** - This defines which groups of recipients should be included in the campaign.
* **Anonymize results when the campaign completes** - When checked, Gophish permanently removes recipient-identifying data from the campaign's results as soon as it completes. See [Anonymizing a Campaign](#anonymizing-a-campaign) for details.

### Scheduling Campaigns

Gophish supports scheduling campaigns, making it easy to plan campaigns in advance. There are two fields to consider when scheduling campaigns: the **Launch Date** and the **Send Emails By** date.

The **Launch Date** is when Gophish should start sending emails. By default, Gophish assumes you want the campaign to be launched immediately.

Gophish also assumes that you want all emails to be sent immediately after the campaign is launched, and to be sent as quickly as possible. However, there are times where you may wish to spread the emails over a period of time. Setting the **Send Emails By** date tells Gophish to spread emails evenly between the launch date and this date. 

### Launching the Campaign

After you have the campaign configuration ready to go, click the "Launch Campaign" button, click through the confirmation message, and you're good to go! Depending on your scheduling settings, Gophish will either launch the campaign immediately or will schedule the campaign to be launched at a later date.

## Viewing Campaign Results

When a campaign is launched, you are automatically redirected to the campaign results screen:

![Campaign results overview](../assets/screen_campaign_result.png)

On the results page, you will see overview information on the campaign status as well as detailed results for each target.

### Exporting Campaign Results

To export campaign results in CSV format, click the "Export CSV" format and select the type of results you want to export:

* **Results** - The current status for each target in the campaign.

  Contains the following fields:

  ```text
  id, email, first_name, last_name, position, status, ip, latitude, longitude
  ```

* **Raw Events** - Contains a stream of events as they occurred during the campaign.

### Completing a Campaign

To complete a campaign, click the "Complete" button and confirm that you want to mark the campaign as completed.

### Anonymizing a Campaign

Some organizations - particularly public institutions bound by data-protection rules - are not permitted to analyze phishing results on a per-person basis. To support these use cases, Gophish-NG can **anonymize** (or "scrub") a campaign, permanently removing recipient-identifying data while keeping the aggregate metrics needed for reporting.

When a campaign is anonymized, Gophish removes recipient-identifying data from its results and timeline:

* The recipient's name and position are replaced with a generic "Anonymized" placeholder.
* The recipient's email address is replaced with a stable, non-identifying pseudonym (for example, `anonymized-a1b2c3d`). Each recipient keeps a distinct pseudonym, so their individual timeline of events is preserved while their real identity is gone.
* The IP address and geolocation recorded for each recipient are removed.
* Any data submitted to the landing page (for example, captured credentials) is removed from the event details.

The aggregate outcome data is preserved, so open rates, click rates, report rates, and event timing remain intact. The browser user-agent recorded for each event is also kept, since it carries no direct personal identifier.

There are two ways to trigger anonymization:

* **Automatically on completion** - Check the "Anonymize results when the campaign completes" option when creating the campaign. Gophish scrubs the results as part of marking the campaign complete.
* **On demand** - Open the campaign's results page and click the "Anonymize" button, then confirm. Once a campaign has been anonymized, the results page shows an "Anonymized" badge.

> Note: Anonymizing a campaign **cannot** be undone. Only the campaign's own results and timeline are affected - the groups used to launch the campaign are left untouched, so any group built from the same recipients still contains their details.

### Deleting a Campaign

To delete a campaign, click the "Delete" button and confirm that you want to delete the campaign.

> Note: This **cannot** be undone, so be careful when deleting a campaign!

### Viewing Result Details

Gophish makes it easy to view the campaign results in a timeline format.

To view the timeline for each recipient, expand the row with the recipient's name.

![Recipient result timeline](../assets/screen_timeline.png)

The results pane shows what a campaign recipient did, such as opening the email, clicking the link, or attempting to submit data from the landing page.

Gophish also records information about the device that clicked the link or submitted data. This data is parsed from the browser's user-agent string. The operating system and browser version is displayed below the event details.

#### Viewing Captured Credentials

If you selected the "Capture Credentials" option when building a landing page, Gophish displays the credentials in the results pane. To view them, click the "View Details" dropdown which renders the captured credentials in a table.

