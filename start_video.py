import os
import json
import socket
import subprocess

def start_video(link, salt:str, args:list=[]):
    print(f"SALT IS {salt}")

    args_str = ' '.join(args)
    
    # Build the complete command string
    command = f"mpv {args_str} --input-ipc-server=/tmp/mpvsocket{salt} {link}"

    subprocess.Popen(['alacritty', '-e', 'bash', '-c', command])
    # os.system()
    # Path to the MPV socket
    # socket_path = f'/tmp/mpvsocket{salt}'
    # Connect to the MPV IPC socket
    # client = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    # client.connect(socket_path)

# start_video("https://video.wixstatic.com/video/36bbae_bef5207b465447f19c0f9366f3ee1c27/720p/mp4/file.mp4", "10", ['--start=100'])