# ZDRCT

This program is designed to spawn some monsters in front of the streamer.

[Demo gif](https://real.kmeaw.com/zdrct.gif)

# Supported environments

* Windows (x86_64)
* GNU/Linux (i686 or x86_64)

ZDoom-based engines should work: the ZDoom itself, GZDoom and Zandronum.

# How does it work?

zdrct communicates both with Twitch and ZDoom. When an event occurs in Twitch, it is handled by the script, which is capable of invoking ZDoom's console commands.

There are three types of events - chat-commands (someone sends a message prefixed with '!'), buttons (so you can have them if you still don't have a Twitch Affilate status) and channel custom reward redemptions (someone spends channel points).

The script is written by the user in [Anko](https://github.com/mattn/anko/tree/master/_example/scripts) language. Chat-commands are represented by cmd_xxxx functions, which are called by zdrct when someone writes !xxxx in the chat. Viewers can supply arguments to these functions: "!hello world 1234" would call cmd_hello("world", "1234").

The script can inteface ZDoom using the function "rcon".

To use the alert system you need to display the web page http://localhost:8666/alerts over your stream (OBS has a built-in browser for a such purpose).

# Quick start

If you are using Windows, download and run the installer. GNU/Linux users are supposed to already know how to build applications from the source (see shell.nix for the list of dependencies).

When you start zdrct it opens a web-browser with a few tabs.

## Tabs

### Twitch

Here you should connect the program with two Twitch accounts - the broadcaster's account (to manipulate rewards and get the channel reference) and a bot's account (to participate in the channel's chat). So click the first "connect" button using your normal Twitch account, then open an incognito window and open the same page to click the second "connect" button - then authorize as a bot on Twitch. This procedure will generate OAuth tokens which will be stored in your profile directory so you would not have to do this once again if you restart the program.

Also you can see your custom rewards and manipulate them for debugging purposes.

### Script

After you establish the connection, click the "Start bot" button at the bottom of this page - it would compile the script and handle the events.

### Actors, Buttons and Rewards

Skip these tabs for now. You can use them to customize the script without manually writing any code - they would generate actor descriptions, button and custom reward manipulation code.

### Doom exe and args

Put the path to your ZDoom-based engine to the top text field and change arguments as you like, then click "Run". It would run the game in a special environment which allows zdrct to control the engine. On Windows an additional console window will appear - don't panic, this is the expected behavior.

### RCon

And the last tab connects zdrct to the engine. Don't change anything and simply click the "Set" button. It should change the status from "offline" to "online" and provide you a test facility input. You can try entering any console command you want (try "say hello") and click "go" - when the game's window gets focused the command should be handled.

If you have completed these steps then everything should be working. Try out some commands in the chat (start with "!help") and redeem some custom rewards. Feel free to experiment with the script to make your own features.

# Scripting language entities reference

## Variables

### admin
The name of the broadcaster

## Functions

### reply(fmt, args...)
Writes a message to the chat

### list_cmds()
Shows the list of chat-commands

### from()
Returns the name of the user which caused this function call

### is_reward()
Returns if the event was caused by channel points redemptions

### rcon(fmt, args...)
Calls ZDoom's console command. Returns true if the command call message was successfully delivered to the ZDoom instance; returns false otherwise.

### sleep(n)
Sleeps for n seconds. n can be int64 or float64.

### alert(message[, image[, sound]])
Shows an alert

### roll(p1, v1, p2, v2, ...)
Returns v1 with a probability of p1, v2 with a probability of p2, ...

### set_last(key, delta)
Stores the moment of delta seconds in the future in the recent-map as key

### last(key)
Gets the key moment from the recent-map (or 0 if it was never stored)

### rate(key, delta)
Returns true and stores the moment of delta seconds in the future in the recent-map as key unless it was stored and haven't been passed yet.
Returns false otherwise.
It is meant to be used as a rate-limiting facility.

### int(s)
Coerces s to an integer. Returns -1 if it fails.

### join(elems, sep)
Returns elems[0] + sep + elems[1] + sep + ... + sep + elems[N-1]

### rand
Returns a floating-point number between 0.0 and 1.0, excluding 1.0.

### randn(n)
Returns an integer between 0 and n, excluding n.

### sprintf(fmt, args...)
Formats args according to the fmt format specifier and returns the resulting string.

### eval(code)
Evaluates Anko code and returns the result value.

### balance(user)
Returns the internal balance of the specified user.

### set_balance(user, new_balance)
Updates the internal balance of the specified user to the specified amount.

## Data types

### int64
Integer

### float64
Floating-point number

### string
An UTF-8 string

### func(...)...
Function (possibly taking arguments) possibly returning results

### bool
Boolean type - true or false

### nil
Unit type, which has exactly one value - nil

### Command
A button. Holds strings Cmd, Text, Image.
Cmd - a name of the chat-function.
Text - a label of the button.
Image - an image.

### Actor
ZDoom's actor. Holds strings ID, AlertImage, AlertSound and Reply.
ID - name of the [class](https://zdoom.org/wiki/Classes)
Name - human-readable name
AlertImage - an image for the alert
AlertSound - a sound for the alert
Reply - a message to be sent by the bot

### Reward
Twitch's custom reward. Hold [a handful of data fields](https://dev.twitch.tv/docs/api/reference#create-custom-rewards)
Most important ones:
Title - the name of a custom reward (string);
Cost - cost of the custom reward (a positive integer);
IsEnabled - if the custom reward is enabled (boolean).

## Special functions to manipulate buttons and rewards
These functions are called once upon the script evaluation. They cannot be called by user events.

### add_command(command)
Adds a button command.

### map_reward(reward, command)
Adds a custom reward, which will call command upon the redemption.
