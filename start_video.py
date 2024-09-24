import os
import json
import socket
import subprocess

def start_video(link, salt:str, args:list=[]):
    # print(f"SALT IS {salt}")

    args_str = ' '.join(args)
    
    # Build the complete command string
    command = f"mpv {args_str} --no-terminal --really-quiet --input-ipc-server=/tmp/mpvsocket{salt} {link}"

    subprocess.Popen(command, shell=True)
    # os.system()
    # Path to the MPV socket
    # socket_path = f'/tmp/mpvsocket{salt}'
    # Connect to the MPV IPC socket
    # client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    # client.connect(socket_path)

def send_command(ipc_socket_path, command):
    """
    Sends a command to the MPV IPC socket and returns the response.
    """
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as s:
        s.connect(ipc_socket_path)
        command_str = json.dumps({"command": command}) + "\n"
        s.sendall(command_str.encode())
        response = s.recv(4096).decode()

        try:
            response_data = json.loads(response)
            if 'data' in response_data:
                return response_data['data']
        except json.JSONDecodeError:
            return None
    return None

def get_mpv_paused_status(ipc_socket_path):
    status = send_command(ipc_socket_path, ["get_property", "pause"])
    if status is not None:
        return status
    else:
        return False

def get_mpv_playback_speed(ipc_socket_path):
    current_speed = send_command(ipc_socket_path, ["get_property", "speed"])
    if current_speed is not None:
        return current_speed
    else:
        print("Failed to get playback speed.")


def get_percentage_watched(ipc_socket_path):
    """
    Calculates the percentage watched of the currently playing video.
    """
    # Get current playback time and total duration
    current_time = send_command(ipc_socket_path, ["get_property", "time-pos"])
    duration = send_command(ipc_socket_path, ["get_property", "duration"])

    if current_time is not None and duration is not None and duration > 0:
        percentage_watched = (current_time / duration) * 100
        return percentage_watched
    return None

def percentage_watched(playback_time:int, duration:int):
    if playback_time is not None and duration is not None and duration > 0:
        video_percentage_watched = (playback_time/duration) * 100
        return video_percentage_watched
    return None

# start_video("https://video.wixstatic.com/video/36bbae_bef5207b465447f19c0f9366f3ee1c27/720p/mp4/file.mp4", "10", ['--start=100'])