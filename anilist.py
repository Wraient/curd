import requests
import json


def get_user_data():
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
  return response.json()


def search_anime_by_title(json_data, search_title):
    """
    Search for an anime by its title in the giv en JSON data and return its ID.

    :param json_data: The JSON data as a dictionary.
    :param search_title: The title of the anime to search for.
    :return: The ID of the anime if found, otherwise None.
    """
    # Navigate to the list of entries
    anime_list = json_data['data']['MediaListCollection']['lists'][0]['entries']
    
    # Search for the anime with the given title
    for entry in anime_list:
        media = entry['media']
        title_romaji = media['title']['romaji']
        title_english = media['title']['english']
        
        # Check if either title matches the search title
        if search_title.lower() in title_romaji.lower() or search_title.lower() in title_english.lower():
            return media['id']
    
    # If not found, return None
    return None

if __name__ == '__main__':
  # Load JSON data from a file (replace 'your_file.json' with the actual file name)
  with open('response.json', 'r') as file:
      data = json.load(file)

  # Define the title to search for
  search_title = 'The Quintessential Quintuplets 2'  # Replace with the actual title you're searching for

  # Search for the anime and get its ID
  anime_id = search_anime_by_title(data, search_title)

  # Print the result
  if anime_id:
      print(f"Anime ID: {anime_id}")
  else:
      print("Anime not found.")
