import kopf

# Tell Kopf to watch for our specific CRD group, version, and plural name
@kopf.on.create('demo.crd', 'v1', 'greetings')
@kopf.on.update('demo.crd', 'v1', 'greetings')
@kopf.on.delete('demo.crd', 'v1', 'greetings')
def greeting_handler(spec, name, namespace, **kwargs):

    name = spec.get('name')
    language = spec.get('language')

    if language == 'English':
        print(f"👋 Hello {name}! Your Greeting resource '{name}' was processed.")
    elif language == 'Spanish':
        print(f"👋 ¡Hola {name}! Your Greeting resource '{name}' was processed.")
    else:
        print(f"👋 Greetings {name}! (Language: {language})")

    return {'message': f"Successfully greeted {name}"}

# It will use your existing ~/.kube/config (the same one kubectl uses) to securely connect to your cluster.