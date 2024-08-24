import requests

# Replace with your actual access token and user ID
access_token = ''
user_id = '6719526'

# GraphQL query
query = """
{
  MediaListCollection(userId: %s, type: ANIME) {
    lists {
      entries {
        media {
          id
          title {
            romaji
            english
          }
        }
        status
        score
        progress
      }
    }
  }
}
""" % user_id

# Send the request
response = requests.post(
    'https://graphql.anilist.co',
    json={'query': query},
    headers={'Authorization': f'Bearer {access_token}'}
)

# Print the response
print(response.json())
